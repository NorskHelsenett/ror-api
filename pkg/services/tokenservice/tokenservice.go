package tokenservice

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient"
	"github.com/NorskHelsenett/ror/pkg/helpers/fouramhelper"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// TODO:
// 1. Move private key to secure storage OK
// 2. Implement key rotation OK
// 3. Implement support for multiple oidc providers and client ids with check on domain name

const (
	VAULT_PATH = "secret/data/v1.0/ror/config/token"
)

var (
	oidcProviderURL string = "https://auth.sky.nhn.no/dex"
	oidcClientId    string = "clusterauth"
	keyStorage      KeyStorage
)

type VaultStorageAdapter struct {
	vaultclient *vaultclient.VaultClient
	secretPath  string
}

func NewVaultStorageAdapter(vaultclient *vaultclient.VaultClient, secretPath string) *VaultStorageAdapter {
	return &VaultStorageAdapter{
		vaultclient: vaultclient,
		secretPath:  secretPath,
	}
}

func (v *VaultStorageAdapter) Set(ks *KeyStorage) error {
	if v.vaultclient == nil {
		return errors.New("vault client not initialized")
	}

	payload, err := json.Marshal(ks)
	if err != nil {
		return err
	}

	data := map[string]interface{}{
		"data": map[string]interface{}{
			"config": string(payload),
		},
	}

	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = v.vaultclient.SetSecret(v.secretPath, body)
	return err
}

func (v *VaultStorageAdapter) Get() (KeyStorage, error) {
	if v.vaultclient == nil {
		return KeyStorage{}, errors.New("vault client not initialized")
	}
	secretData, err := v.vaultclient.GetSecret(v.secretPath)
	if err != nil {
		return KeyStorage{}, err
	}
	data, ok := secretData["data"].(map[string]interface{})

	dataStr, ok := data["config"].(string)
	if !ok {
		return KeyStorage{}, errors.New("invalid data format in vault secret")
	}
	var ks KeyStorage
	err = json.Unmarshal([]byte(dataStr), &ks)
	if err != nil {
		return KeyStorage{}, err
	}
	return ks, nil
}

type StorageProvider interface {
	Set(*KeyStorage) error
	Get() (KeyStorage, error)
}

type KeyStorage struct {
	storageProvider  StorageProvider
	LastRotation     time.Time     `json:"last_rotation"`
	RotationInterval time.Duration `json:"rotation_interval"`
	NumKeys          int           `json:"num_keys"`
	Keys             map[int]Key   `json:"keys"`
}

type Key struct {
	KeyID        string          `json:"key_id"`
	PrivateKey   *rsa.PrivateKey `json:"private_key"`
	AlgorithmKey string          `json:"algorithm_key"`
}

func (k *KeyStorage) GetCurrentKey() Key {
	return k.Keys[1]
}

func (k *KeyStorage) Save() error {
	if k.storageProvider != nil {
		return k.storageProvider.Set(k)
	}
	return errors.New("no storage provider set")
}

func (k *KeyStorage) Load() error {
	if k.storageProvider != nil {
		loaded, err := k.storageProvider.Get()
		if err != nil {
			return err
		}
		k.LastRotation = loaded.LastRotation
		k.RotationInterval = loaded.RotationInterval
		k.NumKeys = loaded.NumKeys
		k.Keys = loaded.Keys
		return nil
	}
	return errors.New("no storage provider set")
}

func Rotate() {
	if keyStorage.needRotate(false) {

		randomInterval, err := rand.Int(rand.Reader, big.NewInt(5000))
		if err != nil {
			rlog.Error("could not generate random interval for key rotation", err)
			return
		}
		time.Sleep(time.Duration(time.Duration(randomInterval.Int64()) * time.Millisecond))
		err = keyStorage.Load()
		if err != nil {
			rlog.Error("could not load keystorage from vault", err)
			return
		}
		rotated := keyStorage.rotate(true)
		if rotated {
			err := keyStorage.Save()
			if err != nil {
				rlog.Error("could not save keystorage to vault", err)
			}
		}
		rlog.Info("Key rotation completed")
	}
}

func (k *KeyStorage) rotate(force bool) bool {
	if k.needRotate(force) {
		for i := 0; i < k.NumKeys; i++ {
			k.Keys[i] = k.Keys[i+1]
			if k.Keys[i].KeyID == "" {
				rlog.Info("generating new key for position", rlog.Int("position", i))
				newKey, err := GenerateKey()
				if err != nil {
					rlog.Error("could not generate new key", err)
				}
				k.Keys[i] = newKey
			}
		}
		k.LastRotation = time.Now()
		return true
	}
	return false
}

func (k *KeyStorage) needRotate(force bool) bool {
	return time.Now().Unix() > k.LastRotation.Add(k.RotationInterval).Unix() || force
}

func GenerateKey() (Key, error) {
	newPrivatekey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return Key{}, err
	}

	thumprint, err := jwk.FromRaw(newPrivatekey.PublicKey)
	if err != nil {
		return Key{}, err
	}
	keyid, err := thumprint.Thumbprint(crypto.SHA256)
	if err != nil {
		return Key{}, err
	}
	key := Key{
		KeyID:        fmt.Sprintf("%x", keyid),
		PrivateKey:   newPrivatekey,
		AlgorithmKey: "RS512",
	}
	return key, nil
}

func Init() {
	keyStorage.storageProvider = NewVaultStorageAdapter(apiconnections.VaultClient, VAULT_PATH)
	err := keyStorage.Load()
	if err != nil {
		rlog.Error("could not load keystorage from vault", err)
	}
	Rotate()
}

// ExchangeToken exchanges a token for a new resigned token
// 1 . Verifies the provided token
// 2. Extracts user information from the token
// 3. (Optional) Checks if the user has admin privileges if admin is true
// 4. Generates and returns a new token for the specified clusterID
func ExchangeToken(ctx context.Context, clusterID string, token string, admin bool) (string, error) {

	provider, err := oidc.NewProvider(ctx, oidcProviderURL)
	if err != nil {
		return "", err
	}
	idTokenVerifier := provider.Verifier(&oidc.Config{
		ClientID: oidcClientId,
	})

	// Parse and verify ID Token payload.
	idToken, err := idTokenVerifier.Verify(ctx, token)
	if err != nil {
		return "", err
	}

	// Extract custom user.
	user := identitymodels.User{Groups: []string{"NotAuthorized"}}
	if err := idToken.Claims(&user); err != nil {
		return "", err
	}

	groupsWithDomain, err := ExtractGroups(&user)
	if err != nil {
		return "", err
	}

	user.Groups = groupsWithDomain

	if admin {
		user.Groups = append(user.Groups, "cluster-admin@ror.io")
	}

	exp := fouramhelper.FourAm()

	newtoken := jwt.NewWithClaims(jwt.GetSigningMethod(keyStorage.GetCurrentKey().AlgorithmKey), jwt.MapClaims{
		"sub":              user.Email,
		"iss":              "https://auth.ror.nhn.no",
		"email":            user.Email,
		"groups":           user.Groups,
		"nbf":              time.Now().Add(-1 * time.Minute).Unix(),
		"iat":              time.Now().Unix(),
		"exp":              exp.Unix(),
		"aud":              oidcClientId,
		"clusterID":        clusterID,
		"providerISS":      user.Issuer,
		"providerAudience": user.Audience,
	})
	newtoken.Header["kid"] = keyStorage.GetCurrentKey().KeyID
	signed, err := newtoken.SignedString(keyStorage.GetCurrentKey().PrivateKey)
	if err != nil {
		return "", err
	}

	return signed, nil
}

// Function extracts groups from user object
func ExtractGroups(user *identitymodels.User) ([]string, error) {
	if user == nil {
		msg := "user is nil"
		rlog.Debug(msg)
		return make([]string, 0), errors.New(msg)
	}

	emailArray := strings.Split(user.Email, "@")
	if len(emailArray) > 2 {
		msg := "could not extract domain from email"
		rlog.Debug(msg)
		return make([]string, 0), errors.New(msg)
	}

	domain := emailArray[1]
	groups := make([]string, 0)
	for i := 0; i < len(user.Groups); i++ {
		g := fmt.Sprintf("%s@%s", user.Groups[i], domain)
		groups = append(groups, g)
	}

	return groups, nil
}

// GetJwks returns the JSON Web Key Set (JWKS) containing the public keys
func GetJwks() (jwk.Set, error) {

	set := jwk.NewSet()
	for _, data := range keyStorage.Keys {
		pubKey := data.PrivateKey.Public().(*rsa.PublicKey)
		jwkKey, err := jwk.FromRaw(pubKey)
		if err != nil {
			return nil, err
		}
		if err := jwkKey.Set(jwk.KeyIDKey, data.KeyID); err != nil {
			return nil, err
		}
		if err := jwkKey.Set(jwk.AlgorithmKey, data.AlgorithmKey); err != nil {
			return nil, err
		}
		if err := jwkKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
			return nil, err
		}

		if err := set.AddKey(jwkKey); err != nil {
			return nil, err
		}
	}

	return set, nil
}
