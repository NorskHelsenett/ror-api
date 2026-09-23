package aclservice

import (
	"context"
	"testing"

	"github.com/NorskHelsenett/ror-api/internal/models"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clusterIdentity is a non-user principal. Before roadmap 2e the ACL delete
// functions rejected it outright; now authorization is the ror:<verb> capability
// on {ror, acl} (enforced by the controllers), so the service functions are not
// identity-type gated.
func clusterIdentity() *identitymodels.Identity {
	return &identitymodels.Identity{
		Type:            identitymodels.IdentityTypeCluster,
		ClusterIdentity: &identitymodels.ServiceIdentity{Id: "c1", Uid: "c1"},
	}
}

// stubAudit swaps the audit sink (which otherwise reaches MongoDB) for a no-op.
func stubAudit(t *testing.T) {
	t.Helper()
	prev := auditCreate
	auditCreate = func(_ context.Context, _ string, _ models.AuditCategory, _ models.AuditAction, _ models.AuditActor, _ any, _ any) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { auditCreate = prev })
}

func TestDeleteV3_NonUserIdentity_NotTypeGated(t *testing.T) {
	setAclStore(t, fakeAclStore{})
	stubAudit(t)

	ok, _, err := DeleteV3(context.Background(), "some-id", clusterIdentity())
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestDeleteV1_NonUserIdentity_NotTypeGated(t *testing.T) {
	setAclStore(t, fakeAclStore{})
	stubAudit(t)

	ok, _, err := Delete(context.Background(), "some-id", clusterIdentity())
	require.NoError(t, err)
	assert.True(t, ok)
}
