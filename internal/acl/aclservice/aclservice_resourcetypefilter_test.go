package aclservice

import (
	"testing"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// testConfigKind is the only protected resource kind today (protected by
// CapRorConfig). The read type-filter must exclude it from callers lacking
// ror:config:read.
const testConfigKind = "Config"

func grant(group string, scope aclscope.Scope, subject aclscope.Subject, access ...aclmodels.AccessTypeV3) aclmodels.AclV3ListItem {
	return aclmodels.AclV3ListItem{Version: 3, Group: group, Scope: scope, Subject: subject, Access: access}
}

// excludedKinds extracts the typemeta.kind $nin list from a type filter, or nil
// when the filter is empty (unrestricted).
func excludedKinds(t *testing.T, filter bson.M) []string {
	t.Helper()
	if len(filter) == 0 {
		return nil
	}
	match, ok := filter["$match"].(bson.M)
	require.True(t, ok, "filter must contain a $match stage: %v", filter)
	kind, ok := match["typemeta.kind"].(bson.M)
	require.True(t, ok, "$match must scope typemeta.kind: %v", match)
	nin, ok := kind["$nin"].([]string)
	require.True(t, ok, "typemeta.kind must use $nin []string: %v", kind)
	return nin
}

// A type-level grant ({ror, Config}) — the shape held by *@cluster.ror.system —
// must admit Config. This is the resolution that a (ror, globalscope) lookup
// would have missed.
func TestResourceTypeReadFilter_TypeLevelGrant_AdmitsConfig(t *testing.T) {
	setResolver(t, aclmodels.AclV3List{
		grant("clusterfleet", aclscope.ScopeRor, aclscope.Subject(testConfigKind),
			aclmodels.CapRor.WithVerb(aclmodels.VerbRead),
			aclmodels.CapRorConfig.WithVerb(aclmodels.VerbRead)),
	}, nil)

	filter, err := ResourceTypeReadFilter(userContext("clusterfleet"))
	require.NoError(t, err)
	assert.Empty(t, excludedKinds(t, filter), "type-level {ror, Config} grant must admit Config")
}

// A global (ror/globalscope) grant with ror:config:read must also admit Config.
func TestResourceTypeReadFilter_GlobalScopeGrant_AdmitsConfig(t *testing.T) {
	setResolver(t, aclmodels.AclV3List{
		grant("admins", aclscope.ScopeRor, aclscope.SubjectGlobal,
			aclmodels.CapRorConfig.WithVerb(aclmodels.VerbRead)),
	}, nil)

	filter, err := ResourceTypeReadFilter(userContext("admins"))
	require.NoError(t, err)
	assert.Empty(t, excludedKinds(t, filter), "{ror, globalscope} grant must admit Config")
}

// A caller holding ror:read but not ror:config:read must have Config excluded.
func TestResourceTypeReadFilter_NoConfigCapability_ExcludesConfig(t *testing.T) {
	setResolver(t, aclmodels.AclV3List{
		grant("viewers", aclscope.ScopeRor, aclscope.SubjectGlobal,
			aclmodels.CapRor.WithVerb(aclmodels.VerbRead)),
	}, nil)

	filter, err := ResourceTypeReadFilter(userContext("viewers"))
	require.NoError(t, err)
	assert.Equal(t, []string{testConfigKind}, excludedKinds(t, filter))
}

// No matching grants at all -> Config excluded.
func TestResourceTypeReadFilter_NoGrants_ExcludesConfig(t *testing.T) {
	setResolver(t, aclmodels.AclV3List{}, nil)

	filter, err := ResourceTypeReadFilter(userContext("nobody"))
	require.NoError(t, err)
	assert.Equal(t, []string{testConfigKind}, excludedKinds(t, filter))
}
