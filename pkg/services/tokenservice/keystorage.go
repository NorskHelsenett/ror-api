package tokenservice

import (
	"crypto/rsa"
	"errors"
	"time"

	"github.com/NorskHelsenett/ror/pkg/rlog"
)

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
