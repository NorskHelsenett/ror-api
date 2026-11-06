package tokenservice

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
)

type jwksSigner struct {
	oidchelper.TokenSigner
	keys jwk.Set
	err  error
}

func (s jwksSigner) GetJWKS() (jwk.Set, error) {
	return s.keys, s.err
}

func TestGetJwks(t *testing.T) {
	previous := signer
	t.Cleanup(func() { signer = previous })

	privateKeys := make(map[string]*rsa.PrivateKey)
	methods := map[string]*jwt.SigningMethodRSA{
		"active": jwt.SigningMethodRS512, "retained": jwt.SigningMethodRS256,
	}
	keys := jwk.NewSet()
	for _, id := range []string{"active", "retained"} {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		privateKeys[id] = privateKey
		key, err := jwk.Import[jwk.Key](privateKey)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{
			jwk.KeyIDKey: id, jwk.AlgorithmKey: methods[id].Alg(), jwk.KeyUsageKey: "sig",
		} {
			if err := key.Set(name, value); err != nil {
				t.Fatal(err)
			}
		}
		if err := keys.AddKey(key); err != nil {
			t.Fatal(err)
		}
	}
	signer = jwksSigner{keys: keys}

	publicKeys, err := GetJwks()
	if err != nil {
		t.Fatal(err)
	}
	if publicKeys.Len() != 2 {
		t.Fatalf("got %d keys, want 2", publicKeys.Len())
	}
	for _, id := range []string{"active", "retained"} {
		key, ok := publicKeys.LookupKeyID(id)
		if !ok {
			t.Fatalf("missing key %q", id)
		}
		if algorithm, ok := key.Algorithm(); !ok || algorithm.String() != methods[id].Alg() {
			t.Fatalf("algorithm not preserved for %q", id)
		}
		if usage, ok := key.KeyUsage(); !ok || usage != "sig" {
			t.Fatalf("usage not preserved for %q", id)
		}
		for _, field := range []string{"d", "p", "q", "dp", "dq", "qi", "oth"} {
			if key.Has(field) {
				t.Fatalf("public key %q contains private field %q", id, field)
			}
		}
		publicKey, err := jwk.Export[*rsa.PublicKey](key)
		if err != nil {
			t.Fatal(err)
		}
		method := methods[id]
		input := "protected-header.payload"
		signature, err := method.Sign(input, privateKeys[id])
		if err != nil {
			t.Fatal(err)
		}
		if err := method.Verify(input, signature, publicKey); err != nil {
			t.Fatalf("%s signature verification failed: %v", method.Alg(), err)
		}
		if err := method.Verify(input+"tampered", signature, publicKey); err == nil {
			t.Fatal("tampered signing input was accepted")
		}
	}
	original, _ := keys.LookupKeyID("active")
	if !original.Has("d") {
		t.Fatal("public projection mutated source private key")
	}
}

func TestGetJwksFailures(t *testing.T) {
	previous := signer
	t.Cleanup(func() { signer = previous })
	sourceErr := errors.New("key storage unavailable")
	symmetric, err := jwk.Import[jwk.Key]([]byte("not-a-public-verification-key"))
	if err != nil {
		t.Fatal(err)
	}
	symmetricKeys := jwk.NewSet()
	if err := symmetricKeys.AddKey(symmetric); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name   string
		signer oidchelper.TokenSigner
		cause  error
	}{
		{name: "uninitialized"},
		{name: "storage error", signer: jwksSigner{err: sourceErr}, cause: sourceErr},
		{name: "nil keyset", signer: jwksSigner{}},
		{name: "empty keyset", signer: jwksSigner{keys: jwk.NewSet()}},
		{name: "symmetric key", signer: jwksSigner{keys: symmetricKeys}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			signer = tt.signer
			keys, err := GetJwks()
			if err == nil || keys != nil {
				t.Fatalf("got (%v, %v), want nil keys and an error", keys, err)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Fatalf("error %v does not wrap %v", err, tt.cause)
			}
		})
	}
}
