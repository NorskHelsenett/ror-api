package aclsnapshot

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclstore"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/tokencontroller"
	"github.com/NorskHelsenett/ror-api/pkg/services/tokenservice"
	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestSnapshotHTTPWithMongoAndJWKS(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("set MONGODB_TEST_URI to a MongoDB test instance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri).
		SetBSONOptions(&options.BSONOptions{ObjectIDAsHexString: true}))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database("acl_snapshot_http_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Drop(cleanup); err != nil {
			t.Error(err)
		}
		if err := client.Disconnect(cleanup); err != nil {
			t.Error(err)
		}
	})
	if err := client.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	_, err = db.Collection("acl").InsertMany(ctx, []any{
		bson.M{"version": 3, "group": aclprincipal.Service(testReader), "scope": "ror", "subject": "acl",
			"access": bson.A{"ror:acl:snapshot:read"}, "issuedBy": "not-in-the-wire-payload"},
		bson.M{"version": 3, "group": aclprincipal.Service(testReader), "scope": "ror", "subject": "acl",
			"access": bson.A{"ror:write", "ror:read"}},
		bson.M{"version": 3, "group": aclprincipal.Service(testReader), "scope": "ror", "subject": "acl",
			"access": bson.A{"ror:acl:snapshot:read"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	storage := &tokenstoragehelper.KeyStorageProvider{Keys: map[int]tokenstoragehelper.Key{
		1: {KeyID: "api-authority", PrivateKey: privateKey, AlgorithmKey: "RS512"},
	}}
	tokenservice.SetManager(oidchelper.NewManagerWithValidator("https://api.example.test", storage, nil))
	t.Cleanup(func() {
		tokenservice.SetManager(oidchelper.NewManagerWithValidator("", &tokenstoragehelper.KeyStorageProvider{}, nil))
	})
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	publisher := aclstore.NewMongoStore(func() *mongo.Database { return db })
	resolver := acl.NewResolver(publisher)
	checkAccess := func(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject, access aclmodels.AccessTypeV3) (bool, error) {
		identity := ctx.Value(identitymodels.ContexIdentity).(identitymodels.Identity)
		groups, err := identity.GetGroups()
		if err != nil {
			return false, err
		}
		return resolver.HasAccess(ctx, groups, scope, subject, access)
	}
	router := gin.New()
	router.Use(Compression())
	router.GET("/v2/token/jwks", tokencontroller.GetJwks())
	router.GET("/v2/acl/snapshot", func(c *gin.Context) {
		c.Set("identity", reader(t, testReader, identitymodels.IdentityProviderApiKey))
		c.Set("timeout", 10*time.Second)
	}, Handler(cfg, publisher, storage.SignPayload, checkAccess))
	fetch := func(path, etag string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-None-Match", etag)
		req.Header.Set("Accept-Encoding", "gzip")
		req = req.WithContext(ctx)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code == http.StatusOK && path == "/v2/acl/snapshot" {
			if response.Header().Get("Content-Encoding") != "gzip" {
				t.Fatal("snapshot transport was not compressed")
			}
			want := fmt.Sprintf("\"%x\"", sha256.Sum256(response.Body.Bytes()))
			if response.Header().Get("ETag") != want {
				t.Fatal("ETag does not identify compressed response bytes")
			}
		}
		if response.Header().Get("Content-Encoding") == "gzip" {
			reader, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			response.Body = bytes.NewBuffer(data)
		}
		return response
	}
	jwksResponse := fetch("/v2/token/jwks", "")
	if jwksResponse.Code != 200 {
		t.Fatalf("JWKS: %d %s", jwksResponse.Code, jwksResponse.Body)
	}
	keys, err := jwk.Parse(jwksResponse.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	key, ok := keys.LookupKeyID("api-authority")
	if !ok {
		t.Fatal("snapshot signing key missing from JWKS")
	}
	publicKey, err := jwk.Export[*rsa.PublicKey](key)
	if err != nil {
		t.Fatal(err)
	}
	first := fetch("/v2/acl/snapshot", "")
	if first.Code != 200 {
		t.Fatalf("snapshot: %d %s", first.Code, first.Body)
	}
	parts := strings.Split(first.Body.String(), ".")
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if err := jwt.SigningMethodRS512.Verify(strings.Join(parts[:2], "."), signature, publicKey); err != nil {
		t.Fatal(err)
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	p, err := snapshot.Decode(payload, cfg.Limits)
	if err != nil || len(p.Rows) != 3 || p.Rows[0].Group != string(aclprincipal.Service(testReader)) || p.Revision != 1 {
		t.Fatalf("unexpected authoritative policy: %#v %v", p, err)
	}
	for i, want := range []string{"ror:acl:snapshot:read", "ror:acl:snapshot:read", "ror:read,ror:write"} {
		var access []string
		for _, value := range p.Rows[i].Grant.Access {
			access = append(access, string(value))
		}
		if strings.Join(access, ",") != want {
			t.Fatalf("repeated grant %d changed: %#v", i, p.Rows[i])
		}
	}
	if strings.Contains(string(payload), "issuedBy") {
		t.Fatal("database metadata leaked into policy")
	}
	etag := first.Header().Get("ETag")
	unchanged := fetch("/v2/acl/snapshot", etag)
	if unchanged.Code != 304 || unchanged.Body.Len() != 0 {
		t.Fatalf("unchanged snapshot: %d %s", unchanged.Code, unchanged.Body)
	}
	if _, err := db.Collection("acl").DeleteMany(ctx, bson.M{"access": bson.A{"ror:write", "ror:read"}}); err != nil {
		t.Fatal(err)
	}
	changed := fetch("/v2/acl/snapshot", etag)
	if changed.Code != 200 || changed.Header().Get("ETag") == etag {
		t.Fatalf("changed snapshot: %d %s", changed.Code, changed.Body)
	}
	parts = strings.Split(changed.Body.String(), ".")
	payload, _ = base64.RawURLEncoding.DecodeString(parts[1])
	p, err = snapshot.Decode(payload, cfg.Limits)
	if err != nil || p.Revision != 2 || len(p.Rows) != 2 {
		t.Fatalf("changed authoritative policy: %#v %v", p, err)
	}
	if _, err := db.Collection("acl").DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatal(err)
	}
	denied := fetch("/v2/acl/snapshot", etag)
	if denied.Code != 403 {
		t.Fatalf("revoked snapshot grant returned %d", denied.Code)
	}
}
