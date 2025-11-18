package tokenservice

import (
	"encoding/json"
	"errors"

	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient"
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
