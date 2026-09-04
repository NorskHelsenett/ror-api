package aclservice

import (
	"context"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func clusterContext(id, uid string) context.Context {
	identity := identitymodels.Identity{
		Type:            identitymodels.IdentityTypeCluster,
		ClusterIdentity: &identitymodels.ServiceIdentity{Id: id, Uid: uid},
	}
	return context.WithValue(context.Background(), identitymodels.ContexIdentity, identity)
}

// TestClusterIdentitySelfAccess verifies a cluster identity has implicit access
// to its own resources addressed by either its cluster id or its uid, and that
// the derived owner filter is keyed by the uid (how resources are stored).
func TestClusterIdentitySelfAccess(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	del := aclmodels.CapRor.WithVerb(aclmodels.VerbDelete)
	const (
		clusterName = "prod-cluster"
		clusterUID  = "22222222-2222-2222-2222-222222222222"
	)

	// The store has no entries; access here is purely the implicit cluster grant.
	setResolver(t, aclmodels.AclV3List{}, nil)
	ctx := clusterContext(clusterName, clusterUID)

	t.Run("own resource by uid is allowed", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), read)
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("own resource by cluster id is allowed", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, clusterName, read)
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("another cluster is denied", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, "someone-else", read)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("delete is not an implicit cluster grant", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), del)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("owner filter is keyed by uid", func(t *testing.T) {
		filter, err := ResourceOwnerFilter(ctx, read)
		require.NoError(t, err)

		match, ok := filter["$match"].(bson.M)
		require.True(t, ok, "expected a $match stage: %v", filter)
		assert.Equal(t, string(aclscope.ScopeCluster), match["rormeta.ownerref.scope"])
		assert.Equal(t, clusterUID, match["rormeta.ownerref.subject"],
			"cluster owner filter must use the uid, not the cluster id")
	})

	t.Run("resolve ownerrefs yields the uid ref", func(t *testing.T) {
		refs, unrestricted, err := ResolveOwnerrefs(ctx, read, acl.OwnerrefFilter{})
		require.NoError(t, err)
		assert.False(t, unrestricted)
		require.Len(t, refs, 1)
		assert.Equal(t, acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: aclscope.Subject(clusterUID)}, refs[0])
	})
}
