package tokencontroller

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/NorskHelsenett/ror-api/pkg/services/tokenservice"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper"
	"github.com/gin-gonic/gin"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

type jwksStorage struct {
	tokenstoragehelper.SigningTokenKeyStorage
	keys jwk.Set
	err  error
}

func (s jwksStorage) GetJwks() (jwk.Set, error) {
	return s.keys, s.err
}

func TestGetJwks(t *testing.T) {
	t.Cleanup(func() {
		tokenservice.SetManager(oidchelper.NewManagerWithValidator("", jwksStorage{}, nil))
	})
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	key, err := jwk.Import[jwk.Key](privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := key.Set(jwk.KeyIDKey, "api-signing-key"); err != nil {
		t.Fatal(err)
	}
	keys := jwk.NewSet()
	if err := keys.AddKey(key); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name   string
		keys   jwk.Set
		err    error
		status int
	}{
		{name: "public verification keys", keys: keys, status: http.StatusOK},
		{name: "storage failure", err: errors.New("storage unavailable"), status: http.StatusInternalServerError},
		{name: "missing keys", status: http.StatusInternalServerError},
		{name: "empty keys", keys: jwk.NewSet(), status: http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := jwksStorage{keys: tt.keys, err: tt.err}
			tokenservice.SetManager(oidchelper.NewManagerWithValidator("https://api.example.test", storage, nil))
			router := gin.New()
			router.GET("/v2/token/jwks", GetJwks())
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v2/token/jwks", nil))
			if response.Code != tt.status {
				t.Fatalf("got status %d, want %d: %s", response.Code, tt.status, response.Body)
			}
			if tt.status != http.StatusOK {
				if response.Header().Get("X-ROR-ERROR") == "" {
					t.Fatal("failure did not use the standard API error response")
				}
				return
			}
			if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("unexpected content type %q", got)
			}
			published, err := jwk.Parse(response.Body.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if published.Len() != 1 {
				t.Fatalf("got %d keys, want 1", published.Len())
			}
			publicKey, ok := published.LookupKeyID("api-signing-key")
			if !ok {
				t.Fatal("missing API signing key")
			}
			for _, field := range []string{"d", "p", "q", "dp", "dq", "qi", "oth"} {
				if publicKey.Has(field) {
					t.Fatalf("response contains private field %q", field)
				}
			}
		})
	}
}
