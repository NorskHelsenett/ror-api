package resourcesv2service

import (
	"testing"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clusterSelfOwnerref() rorresourceowner.RorResourceOwnerReference {
	return rorresourceowner.RorResourceOwnerReference{
		Scope:   aclscope.ScopeCluster,
		Subject: aclscope.Subject(testClusterID),
	}
}

func TestHasProtectedKindWriteAccess_UnprotectedKind_Allowed(t *testing.T) {
	allowed, err := hasProtectedKindWriteAccess(testCtx(), "Pod", clusterSelfOwnerref())
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestHasProtectedKindWriteAccess_ProtectedKind_ClusterDenied(t *testing.T) {
	// Cluster identities never hold ror:config:write, so writing a protected
	// Config resource is denied regardless of ownerref.
	allowed, err := hasProtectedKindWriteAccess(testCtx(), "Config", clusterSelfOwnerref())
	require.NoError(t, err)
	assert.False(t, allowed)
}
