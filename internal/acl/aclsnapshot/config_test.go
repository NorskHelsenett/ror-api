package aclsnapshot

import (
	"strings"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
)

func TestLoadConfig(t *testing.T) {
	values := map[string]string{
		"ACL_SNAPSHOT_MAX_PAYLOAD_BYTES": "8192", "ACL_SNAPSHOT_MAX_ENVELOPE_BYTES": "16384",
		"ACL_SNAPSHOT_MAX_ROWS": "10", "ACL_SNAPSHOT_MAX_ACCESS_ENTRIES": "10",
		"ACL_SNAPSHOT_MAX_FIELD_BYTES": "128", "ACL_SNAPSHOT_MAX_HEADER_BYTES": "256",
	}
	old := map[string]string{}
	for key, value := range values {
		old[key] = rorconfig.GetString(key)
		rorconfig.Set(key, value)
	}
	t.Cleanup(func() {
		for key, value := range old {
			rorconfig.Set(key, value)
		}
	})
	cfg, err := LoadConfig()
	if err != nil || cfg.Limits.MaxRows != 10 {
		t.Fatalf("load: %#v, %v", cfg, err)
	}
	for key, value := range values {
		for _, invalid := range []string{"0", "-1", "invalid", " 10", "99999999999999999999999999"} {
			rorconfig.Set(key, invalid)
			if _, err := LoadConfig(); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("%s=%q: got %v", key, invalid, err)
			}
		}
		rorconfig.Set(key, value)
	}
	rorconfig.Set("ACL_SNAPSHOT_MAX_PAYLOAD_BYTES", "16777216")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("MongoDB document ceiling ignored")
	}
	for key := range values {
		rorconfig.Set(key, "")
	}
	cfg, err = LoadConfig()
	if err != nil || cfg.Limits.MaxPayloadBytes != 8388608 ||
		cfg.Limits.MaxEnvelopeBytes != 12582912 || cfg.Limits.MaxRows != 10000 ||
		cfg.Limits.MaxAccessEntries != 64 || cfg.Limits.MaxFieldBytes != 1024 ||
		cfg.Limits.MaxHeaderBytes != 4096 {
		t.Fatalf("defaults: %#v, %v", cfg, err)
	}
	rorconfig.Set("ACL_SNAPSHOT_MAX_ROWS", "25")
	cfg, err = LoadConfig()
	if err != nil || cfg.Limits.MaxRows != 25 || cfg.Limits.MaxPayloadBytes != 8388608 {
		t.Fatalf("single override: %#v, %v", cfg, err)
	}
}
