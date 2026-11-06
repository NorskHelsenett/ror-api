// Package aclsnapshot publishes authoritative signed ACL snapshots.
package aclsnapshot

import (
	"fmt"
	"strconv"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
)

type Config struct {
	ReaderServiceID string
	Limits          snapshot.Limits
}

// LoadConfig leaves the endpoint disabled when no reader is configured.
// Once enabled, every bound must be explicitly configured.
func LoadConfig() (Config, error) {
	rorconfig.SetDefault("ACL_SNAPSHOT_READER_SERVICE_ID", "")
	cfg := Config{ReaderServiceID: rorconfig.GetString("ACL_SNAPSHOT_READER_SERVICE_ID")}
	if cfg.ReaderServiceID == "" {
		return cfg, nil
	}
	fields := []struct {
		key    string
		target *int
	}{
		{"ACL_SNAPSHOT_MAX_PAYLOAD_BYTES", &cfg.Limits.MaxPayloadBytes},
		{"ACL_SNAPSHOT_MAX_ENVELOPE_BYTES", &cfg.Limits.MaxEnvelopeBytes},
		{"ACL_SNAPSHOT_MAX_ROWS", &cfg.Limits.MaxRows},
		{"ACL_SNAPSHOT_MAX_ACCESS_ENTRIES", &cfg.Limits.MaxAccessEntries},
		{"ACL_SNAPSHOT_MAX_FIELD_BYTES", &cfg.Limits.MaxFieldBytes},
		{"ACL_SNAPSHOT_MAX_HEADER_BYTES", &cfg.Limits.MaxHeaderBytes},
	}
	for _, field := range fields {
		rorconfig.SetDefault(field.key, "")
		value, err := strconv.Atoi(rorconfig.GetString(field.key))
		if err != nil || value <= 0 {
			return Config{}, fmt.Errorf("%s must be explicitly configured as a positive integer", field.key)
		}
		*field.target = value
	}
	// Leave room for BSON publication metadata below MongoDB's 16 MiB limit.
	if cfg.Limits.MaxPayloadBytes > 15*1024*1024 {
		return Config{}, fmt.Errorf("ACL_SNAPSHOT_MAX_PAYLOAD_BYTES exceeds the 15 MiB storage ceiling")
	}
	return cfg, cfg.Limits.Validate()
}
