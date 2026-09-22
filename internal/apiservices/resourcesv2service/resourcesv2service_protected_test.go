package resourcesv2service

import (
	"context"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clusterSelfOwnerref() rorresourceowner.RorResourceOwnerReference {
	return rorresourceowner.RorResourceOwnerReference{
		Scope:   aclscope.ScopeCluster,
		Subject: aclscope.Subject(testClusterID),
	}
}

// configWriterCtx returns a user identity in testConfigWriterGroup, whose ACL
// grant holds ror:config:write.
func configWriterCtx() context.Context {
	identity := identitymodels.Identity{
		Type: identitymodels.IdentityTypeUser,
		User: &identitymodels.User{Email: "writer@e2e.invalid", Groups: []string{testConfigWriterGroup}},
	}
	return context.WithValue(context.Background(), identitymodels.ContexIdentity, identity)
}

func TestHasProtectedKindWriteAccess_UnprotectedKind_Allowed(t *testing.T) {
	allowed, err := hasProtectedKindWriteAccess(testCtx(), "Pod", clusterSelfOwnerref())
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestHasProtectedKindWriteAccess_ProtectedKind_ClusterDenied(t *testing.T) {
	// Cluster identities never hold ror:config:write, so writing a protected
	// Config resource is denied — by the missing grant, not an identity check.
	allowed, err := hasProtectedKindWriteAccess(testCtx(), "Config", clusterSelfOwnerref())
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestHasProtectedKindWriteAccess_ProtectedKind_GrantHolderAllowed(t *testing.T) {
	// An identity whose ACL grant includes ror:config:write may write the
	// protected Config kind: enforcement is grant-driven, not identity-type based.
	owner := rorresourceowner.RorResourceOwnerReference{
		Scope:   aclscope.ScopeRor,
		Subject: aclscope.Subject("Config"),
	}
	allowed, err := hasProtectedKindWriteAccess(configWriterCtx(), "Config", owner)
	require.NoError(t, err)
	assert.True(t, allowed)
}
