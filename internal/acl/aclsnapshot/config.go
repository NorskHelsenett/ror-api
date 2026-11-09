// Package aclsnapshot publishes authoritative signed ACL snapshots.
package aclsnapshot

import (
	"fmt"
	"strconv"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
)

type Config struct {
	Limits snapshot.Limits
}

// LoadConfig supplies defaults and validates optional deployment overrides.
func LoadConfig() (Config, error) {
	cfg := Config{Limits: snapshot.DefaultLimits()}
	defaults := cfg.Limits
	fields := []struct {
		key          string
		target       *int
		defaultValue int
	}{
		{"ACL_SNAPSHOT_MAX_PAYLOAD_BYTES", &cfg.Limits.MaxPayloadBytes, defaults.MaxPayloadBytes},
		{"ACL_SNAPSHOT_MAX_ENVELOPE_BYTES", &cfg.Limits.MaxEnvelopeBytes, defaults.MaxEnvelopeBytes},
		{"ACL_SNAPSHOT_MAX_ROWS", &cfg.Limits.MaxRows, defaults.MaxRows},
		{"ACL_SNAPSHOT_MAX_ACCESS_ENTRIES", &cfg.Limits.MaxAccessEntries, defaults.MaxAccessEntries},
		{"ACL_SNAPSHOT_MAX_FIELD_BYTES", &cfg.Limits.MaxFieldBytes, defaults.MaxFieldBytes},
		{"ACL_SNAPSHOT_MAX_HEADER_BYTES", &cfg.Limits.MaxHeaderBytes, defaults.MaxHeaderBytes},
	}
	for _, field := range fields {
		rorconfig.SetDefault(field.key, field.defaultValue)
		value, err := strconv.Atoi(rorconfig.GetString(field.key))
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("%s must be a positive integer", field.key)
		}
		*field.target = value
	}
	// Leave room for BSON publication metadata below MongoDB's 16 MiB limit.
	if cfg.Limits.MaxPayloadBytes > 15*1024*1024 {
		return Config{}, fmt.Errorf("ACL_SNAPSHOT_MAX_PAYLOAD_BYTES exceeds the 15 MiB storage ceiling")
	}
	return cfg, cfg.Limits.Validate()
}
