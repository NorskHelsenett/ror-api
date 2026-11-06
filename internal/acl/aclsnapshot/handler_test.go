package aclsnapshot

import (
	"bytes"
	stdgzip "compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclgrants"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func testConfig() Config {
	return Config{Limits: snapshot.Limits{
		MaxPayloadBytes: 8192, MaxEnvelopeBytes: 16384, MaxRows: 10,
		MaxAccessEntries: 10, MaxFieldBytes: 128, MaxHeaderBytes: 256,
	}}
}

const testReader = "ror-auth-site-a"

func allowSnapshot(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject, access aclmodels.AccessTypeV3) (bool, error) {
	identity, ok := ctx.Value(identitymodels.ContexIdentity).(identitymodels.Identity)
	return ok && identity.Validate() == nil && scope == aclscope.ScopeRor &&
		subject == aclscope.SubjectAcl && access == aclmodels.CapRorAclSnapshot.WithVerb(aclmodels.VerbRead), nil
}

type snapshotGrants struct {
	grants aclgrants.Grants
	err    error
}

func (s snapshotGrants) GetGrants(context.Context, []string) (aclgrants.Grants, error) {
	return s.grants, s.err
}

func TestSnapshotCapabilityAuthorization(t *testing.T) {
	cfg := testConfig()
	payload, err := snapshot.Encode(snapshot.Payload{
		Schema: snapshot.Schema, Revision: 1, GeneratedAt: time.Unix(1, 0).UTC(), Rows: []snapshot.Row{},
	}, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := &tokenstoragehelper.KeyStorageProvider{Keys: map[int]tokenstoragehelper.Key{
		1: {KeyID: "authorized-snapshot", PrivateKey: privateKey, AlgorithmKey: "RS512"},
	}}
	user, err := identitymodels.NewUserIdentity(identitymodels.AuthInfo{AuthProvider: identitymodels.IdentityProviderOidc},
		"reader@example.test", "Reader", []string{"readers"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cluster, err := identitymodels.NewClusterIdentity(identitymodels.AuthInfo{AuthProvider: identitymodels.IdentityProviderApiKey}, "cluster", "cluster-uid")
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range []identitymodels.Identity{
		user, cluster, reader(t, testReader, identitymodels.IdentityProviderApiKey),
		reader(t, "another-service", identitymodels.IdentityProviderOidc),
	} {
		for _, tt := range []struct {
			name   string
			grant  aclgrants.Grant
			err    error
			status int
		}{
			{"snapshot read", aclgrants.Grant{Scope: "ror", Subject: "acl", Access: []aclmodels.AccessTypeV3{"ror:acl:snapshot:read"}}, nil, 200},
			{"no access", aclgrants.Grant{Scope: "ror", Subject: "acl", Access: []aclmodels.AccessTypeV3{}}, nil, 403},
			{"ordinary ACL read", aclgrants.Grant{Scope: "ror", Subject: "acl", Access: []aclmodels.AccessTypeV3{"ror:read"}}, nil, 403},
			{"wrong subject", aclgrants.Grant{Scope: "ror", Subject: "Config", Access: []aclmodels.AccessTypeV3{"ror:acl:snapshot:read"}}, nil, 403},
			{"wrong scope", aclgrants.Grant{Scope: "Project", Subject: "acl", Access: []aclmodels.AccessTypeV3{"ror:acl:snapshot:read"}}, nil, 403},
			{"resolver error", aclgrants.Grant{}, errors.New("store unavailable"), 500},
		} {
			t.Run(identity.GetId()+"/"+tt.name, func(t *testing.T) {
				resolver := acl.NewResolverWithGrantSource(snapshotGrants{aclgrants.Grants{tt.grant}, tt.err})
				check := func(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject, access aclmodels.AccessTypeV3) (bool, error) {
					got := ctx.Value(identitymodels.ContexIdentity).(identitymodels.Identity)
					groups, err := got.GetGroups()
					if err != nil {
						return false, err
					}
					return resolver.HasAccess(ctx, groups, scope, subject, access)
				}
				publisher := &fakePublisher{payload: payload}
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set("identity", identity) })
				router.GET("/v2/acl/snapshot", Handler(cfg, publisher, keys.SignPayload, check))
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v2/acl/snapshot", nil))
				if recorder.Code != tt.status {
					t.Fatalf("status %d, want %d", recorder.Code, tt.status)
				}
				wantCalls := 0
				if tt.name == "snapshot read" {
					wantCalls = 1
				}
				if publisher.calls != wantCalls {
					t.Fatalf("published %d times, want %d", publisher.calls, wantCalls)
				}
			})
		}
	}
}

type fakePublisher struct {
	calls   int
	payload []byte
	err     error
	ctx     context.Context
}

func (p *fakePublisher) PublishSnapshot(ctx context.Context, _ snapshot.Limits) ([]byte, error) {
	p.calls++
	p.ctx = ctx
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.payload, p.err
}

func reader(t *testing.T, id string, provider identitymodels.IdentityProvider) identitymodels.Identity {
	t.Helper()
	value, err := identitymodels.NewServiceIdentity(identitymodels.AuthInfo{
		AuthProvider: provider, AuthProviderID: "credential-id",
	}, id)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func request(cfg Config, identity any, publisher Publisher, sign SignPayload, etag string) *httptest.ResponseRecorder {
	return requestEncoding(cfg, identity, publisher, sign, etag, "gzip")
}

func requestEncoding(cfg Config, identity any, publisher Publisher, sign SignPayload, etag, encoding string) *httptest.ResponseRecorder {
	router := gin.New()
	router.Use(Compression())
	router.Use(func(c *gin.Context) {
		if identity != nil {
			c.Set("identity", identity)
		}
		c.Set("timeout", time.Second)
	})
	router.GET("/v2/acl/snapshot", Handler(cfg, publisher, sign, allowSnapshot))
	response := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v2/acl/snapshot", nil)
	req.Header.Set("Accept-Encoding", encoding)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	router.ServeHTTP(response, req)
	return response
}

func TestHandlerSignedSnapshotAndConditionalFetch(t *testing.T) {
	cfg := testConfig()
	payload, err := snapshot.Encode(snapshot.Payload{
		Schema: snapshot.Schema, Revision: 1, GeneratedAt: time.Unix(1, 0).UTC(), Rows: []snapshot.Row{},
	}, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := &tokenstoragehelper.KeyStorageProvider{Keys: map[int]tokenstoragehelper.Key{
		1: {KeyID: "api-key-1", PrivateKey: privateKey, AlgorithmKey: "RS512"},
	}}
	publisher := &fakePublisher{payload: payload}
	identity := reader(t, testReader, identitymodels.IdentityProviderApiKey)
	response := request(cfg, identity, publisher, keys.SignPayload, "")
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/jose" {
		t.Fatalf("unexpected response %d: %s", response.Code, response.Body)
	}
	if response.Header().Get("Content-Encoding") != "gzip" ||
		response.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatal("snapshot was not compressed with content negotiation")
	}
	compressed, err := stdgzip.NewReader(bytes.NewReader(response.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(compressed)
	if err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	plain := requestEncoding(cfg, identity, publisher, keys.SignPayload, "", "identity")
	if plain.Code != 200 || plain.Header().Get("Content-Encoding") != "" ||
		plain.Header().Get("Vary") != "Accept-Encoding" || !bytes.Equal(decoded, plain.Body.Bytes()) {
		t.Fatal("decompression did not restore the identical signed representation")
	}
	for _, res := range []*httptest.ResponseRecorder{response, plain} {
		want := fmt.Sprintf("\"%x\"", sha256.Sum256(res.Body.Bytes()))
		if res.Header().Get("ETag") != want {
			t.Fatalf("ETag does not hash actual representation: got %q, want %q", res.Header().Get("ETag"), want)
		}
	}
	if response.Body.Len() >= plain.Body.Len() {
		t.Fatal("gzip did not reduce snapshot transfer size")
	}
	parts := strings.Split(string(decoded), ".")
	signature, _ := base64.RawURLEncoding.DecodeString(parts[2])
	if err := jwt.SigningMethodRS512.Verify(strings.Join(parts[:2], "."), signature, &privateKey.PublicKey); err != nil {
		t.Fatal(err)
	}
	received, _ := base64.RawURLEncoding.DecodeString(parts[1])
	if string(received) != string(payload) {
		t.Fatal("wire payload changed")
	}
	tag := response.Header().Get("ETag")
	if len(tag) != 66 || !strings.HasPrefix(tag, "\"") {
		t.Fatalf("invalid ETag %q", tag)
	}
	plainTag := plain.Header().Get("ETag")
	if plainTag == tag {
		t.Fatal("compressed and identity representations share a strong ETag")
	}
	for _, tt := range []struct {
		encoding, tag string
		status        int
	}{
		{"identity", plainTag, 304},
		{"gzip", plainTag, 200},
		{"identity", tag, 200},
	} {
		res := requestEncoding(cfg, identity, publisher, keys.SignPayload, tt.tag, tt.encoding)
		if res.Code != tt.status || (res.Code == 304 && res.Body.Len() != 0) {
			t.Fatalf("encoding %s conditional status: %d", tt.encoding, res.Code)
		}
	}
	for _, tt := range []struct {
		header string
		status int
	}{
		{tag, 304}, {`"other", ` + tag, 304}, {`"other"`, 200},
	} {
		got := request(cfg, identity, publisher, keys.SignPayload, tt.header)
		if got.Code != tt.status || got.Header().Get("ETag") != tag {
			t.Fatalf("header %s: got %d: %s", tt.header, got.Code, got.Body)
		}
		if tt.status == 304 && got.Body.Len() != 0 {
			t.Fatal("304 contains a body")
		}
		if got.Header().Get("Vary") != "Accept-Encoding" {
			t.Fatal("conditional response lost content negotiation")
		}
	}
	current := keys.Keys[1]
	current.KeyID = "api-key-2"
	keys.Keys[1] = current
	resigned := request(cfg, identity, publisher, keys.SignPayload, tag)
	if resigned.Code != 200 || resigned.Header().Get("ETag") == tag {
		t.Fatal("changed signing identity did not change representation")
	}
}

func TestHTTPClientDecompressesSignedSnapshot(t *testing.T) {
	cfg := testConfig()
	payload, err := snapshot.Encode(snapshot.Payload{
		Schema: snapshot.Schema, Revision: 1, GeneratedAt: time.Unix(1, 0).UTC(), Rows: []snapshot.Row{},
	}, cfg.Limits)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	keys := &tokenstoragehelper.KeyStorageProvider{Keys: map[int]tokenstoragehelper.Key{
		1: {KeyID: "http-client-key", PrivateKey: privateKey, AlgorithmKey: "RS512"},
	}}
	identity := reader(t, testReader, identitymodels.IdentityProviderApiKey)
	router := gin.New()
	router.Use(Compression(), func(c *gin.Context) { c.Set("identity", identity) })
	router.GET("/v2/acl/snapshot", Handler(cfg, &fakePublisher{payload: payload}, keys.SignPayload, allowSnapshot))
	server := httptest.NewServer(router)
	defer server.Close()
	client := server.Client()
	client.Timeout = 5 * time.Second
	res, err := client.Get(server.URL + "/v2/acl/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 || !res.Uncompressed || res.Header.Get("Content-Encoding") != "" {
		t.Fatal("Go HTTP client did not transparently decompress the response")
	}
	envelope, err := keys.SignPayload(payload, snapshot.MediaType)
	if err != nil || string(data) != envelope {
		t.Fatalf("HTTP client did not restore original signed bytes: %v", err)
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v2/acl/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-None-Match", res.Header.Get("ETag"))
	conditional, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer conditional.Body.Close()
	body, err := io.ReadAll(conditional.Body)
	if err != nil || conditional.StatusCode != 304 || len(body) != 0 ||
		conditional.Header.Get("Vary") != "Accept-Encoding" {
		t.Fatalf("HTTP client conditional fetch: status %d, error %v", conditional.StatusCode, err)
	}
}

func TestHandlerAuthorization(t *testing.T) {
	cfg := testConfig()
	user, err := identitymodels.NewUserIdentity(identitymodels.AuthInfo{AuthProvider: identitymodels.IdentityProviderApiKey},
		"admin@example.test", "Admin", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		identity any
		status   int
	}{
		{"anonymous", nil, 401}, {"invalid identity type", "bad", 401},
		{"user API key", user, 500},
		{"different service", reader(t, "other", identitymodels.IdentityProviderApiKey), 500},
		{"OIDC service", reader(t, testReader, identitymodels.IdentityProviderOidc), 500},
		{"incomplete identity", identitymodels.Identity{}, 401},
	} {
		t.Run(tt.name, func(t *testing.T) {
			publisher := &fakePublisher{err: errors.New("authorized")}
			got := request(cfg, tt.identity, publisher, nil, "")
			wantCalls := 0
			if tt.status == 500 {
				wantCalls = 1
			}
			if got.Code != tt.status || publisher.calls != wantCalls {
				t.Fatalf("got %d, published %d times", got.Code, publisher.calls)
			}
		})
	}
}

func TestHandlerFailures(t *testing.T) {
	cfg := testConfig()
	identity := reader(t, testReader, identitymodels.IdentityProviderApiKey)
	signErr := func([]byte, string) (string, error) { return "", errors.New("signing unavailable") }
	for _, tt := range []struct {
		name      string
		publisher *fakePublisher
		sign      SignPayload
		header    string
		status    int
	}{
		{"invalid conditional", &fakePublisher{}, nil, "unquoted", 400},
		{"wildcard conditional", &fakePublisher{}, nil, "*", 400},
		{"weak conditional", &fakePublisher{}, nil, `W/"tag"`, 400},
		{"oversized conditional", &fakePublisher{}, nil, strings.Repeat("x", 257), 400},
		{"database failure", &fakePublisher{err: errors.New("database unavailable")}, nil, "", 500},
		{"signing failure", &fakePublisher{payload: []byte("{}")}, signErr, "", 500},
		{"bad signed data", &fakePublisher{payload: []byte("{}")}, func([]byte, string) (string, error) { return "bad", nil }, "", 500},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := request(cfg, identity, tt.publisher, tt.sign, tt.header)
			if got.Code != tt.status || got.Header().Get("X-ROR-ERROR") == "" {
				t.Fatalf("got %d: %s", got.Code, got.Body)
			}
		})
	}
}

func TestParseConditional(t *testing.T) {
	for _, value := range []string{`""`, `"a,b"`, `"a" , "b"`, ""} {
		if _, err := parseConditional(value, 256); err != nil {
			t.Fatalf("%q: %v", value, err)
		}
	}
	for _, value := range []string{`"a`, `"a",`, `"a" "b"`, `*, "a"`, "*", `W/"a"`, `w/"a"`, "\"a b\"", "\"a\x7fb\"", "\n"} {
		if _, err := parseConditional(value, 256); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	if _, err := parseConditional(`"abc"`, 5); err != nil {
		t.Fatal(err)
	}
	if _, err := parseConditional(`"abc"`, 4); err == nil {
		t.Fatal("over-limit header accepted")
	}
}

func TestValidateEnvelope(t *testing.T) {
	limits := testConfig().Limits
	payload := []byte("{}")
	valid := func(header string) string {
		return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
			base64.RawURLEncoding.EncodeToString(payload) + ".signature"
	}
	header := `{"alg":"RS512","kid":"key","typ":"application/ror-acl-snapshot+jws"}`
	for _, invalid := range []string{
		"a.b", "a.e30.", "!.e30.x", valid("{"),
		valid(strings.Replace(header, "RS512", "RS256", 1)),
		valid(strings.Replace(header, `"key"`, `""`, 1)),
		valid(strings.Replace(header, `"key"`, `"`+strings.Repeat("x", 129)+`"`, 1)),
		valid(strings.Replace(header, "application/ror-acl-snapshot+jws", "JWT", 1)),
		valid(strings.Repeat("x", 257)), "a.changed.signature",
	} {
		if err := validateEnvelope(invalid, payload, limits); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
	envelope := valid(header)
	limits.MaxEnvelopeBytes = len(envelope)
	limits.MaxHeaderBytes = len(header)
	if err := validateEnvelope(envelope, payload, limits); err != nil {
		t.Fatal(err)
	}
	limits.MaxEnvelopeBytes--
	if err := validateEnvelope(envelope, payload, limits); err == nil {
		t.Fatal("over-limit envelope accepted")
	}
}
