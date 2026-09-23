package aclservice

import (
	"context"
	"testing"

	"github.com/NorskHelsenett/ror-api/internal/mocks/identitymocks"
	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/acl/aclstore"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func clusterContext(id, uid string) context.Context {
	return context.WithValue(context.Background(), identitymodels.ContexIdentity, identitymocks.Cluster(id, uid))
}

// clusterSelfGrant is the ACL data that replaces the old hardcoded implicit
// cluster access: a grant on the cluster's own scope+subject, held by the group
// the cluster identity is a member of.
func clusterSelfGrant(uid string) aclmodels.AclV3ListItem {
	return aclEntry(
		aclprincipal.Cluster(uid),
		aclscope.ScopeCluster,
		aclscope.Subject(uid),
		ClusterSelfAccess()...,
	)
}

// TestClusterIdentitySelfAccess verifies a cluster identity resolves access to
// its own resources through the ordinary group mechanism, and that the derived
// owner filter is keyed by the uid (how resources are stored).
func TestClusterIdentitySelfAccess(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	del := aclmodels.CapRor.WithVerb(aclmodels.VerbDelete)
	const (
		clusterName = "prod-cluster"
		clusterUID  = "22222222-2222-2222-2222-222222222222"
	)

	setResolver(t, aclmodels.AclV3List{clusterSelfGrant(clusterUID)}, nil)
	ctx := clusterContext(clusterName, clusterUID)

	t.Run("own resource by uid is allowed", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), read)
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("another cluster is denied", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, "someone-else", read)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("delete is not granted", func(t *testing.T) {
		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), del)
		require.NoError(t, err)
		assert.False(t, allowed)
	})

	t.Run("owner filter is keyed by uid", func(t *testing.T) {
		filter, err := ResourceOwnerFilter(ctx, read)
		require.NoError(t, err)

		match, ok := filter["$match"].(bson.M)
		require.True(t, ok, "expected a $match stage: %v", filter)
		or, ok := match["$or"].(bson.A)
		require.True(t, ok, "expected an $or in $match: %v", match)

		inClause := or[0].(bson.M)["rormeta.ownerref"].(bson.M)["$in"].(bson.A)
		require.Len(t, inClause, 1)
		entry := inClause[0].(bson.D)
		assert.Equal(t, string(aclscope.ScopeCluster), entry[0].Value)
		assert.Equal(t, clusterUID, entry[1].Value,
			"cluster owner filter must use the uid, not the cluster id")

		assert.Equal(t, bson.M{"uid": bson.M{"$in": bson.A{clusterUID}}}, or[1],
			"uid-self-match must use the cluster uid")
	})

	t.Run("resolve ownerrefs yields the uid ref", func(t *testing.T) {
		refs, unrestricted, err := ResolveOwnerrefs(ctx, read, acl.OwnerrefFilter{})
		require.NoError(t, err)
		assert.False(t, unrestricted)
		require.Len(t, refs, 1)
		assert.Equal(t, acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: aclscope.Subject(clusterUID)}, refs[0])
	})
}

// TestClusterWithoutGrantHasNoAccess asserts cluster access is purely ACL data:
// a cluster whose self grant was never provisioned is denied. This is what the
// backfill migration protects against.
func TestClusterWithoutGrantHasNoAccess(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	const clusterUID = "33333333-3333-3333-3333-333333333333"

	setResolver(t, aclmodels.AclV3List{}, nil)
	ctx := clusterContext("no-grant-cluster", clusterUID)

	allowed, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), read)
	require.NoError(t, err)
	assert.False(t, allowed)

	filter, err := ResourceOwnerFilter(ctx, read)
	require.NoError(t, err)
	assert.Equal(t, aclstore.DenyAllFilter, filter)
}

// TestClusterWithoutUidIsRejected asserts a cluster identity must carry a uid:
// grants are keyed by uid, so a uid-less identity fails closed rather than
// silently falling back to the cluster id. The constructors reject such an
// identity outright, so the struct literal here is the only way one can still
// reach the ACL layer.
func TestClusterWithoutUidIsRejected(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	const clusterUID = "44444444-4444-4444-4444-444444444444"

	setResolver(t, aclmodels.AclV3List{clusterSelfGrant(clusterUID)}, nil)
	ctx := context.WithValue(context.Background(), identitymodels.ContexIdentity,
		identitymodels.Identity{Type: identitymodels.IdentityTypeCluster})

	_, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), read)
	require.Error(t, err)

	filter, err := ResourceOwnerFilter(ctx, read)
	require.Error(t, err)
	assert.Equal(t, aclstore.DenyAllFilter, filter)
}

// TestClusterFleetGrant asserts fleet-wide cluster access is expressed as a
// grant on the aggregate group every cluster belongs to.
func TestClusterFleetGrant(t *testing.T) {
	configRead := aclmodels.CapRorConfig.WithVerb(aclmodels.VerbRead)
	configWrite := aclmodels.CapRorConfig.WithVerb(aclmodels.VerbWrite)
	const clusterUID = "55555555-5555-5555-5555-555555555555"

	setResolver(t, aclmodels.AclV3List{
		clusterSelfGrant(clusterUID),
		aclEntry(
			aclprincipal.AllClusters(),
			aclscope.ScopeRor,
			aclscope.Subject("Config"),
			aclmodels.AccessRorRead, aclmodels.AccessRorConfigRead,
		),
	}, nil)
	ctx := clusterContext("fleet-cluster", clusterUID)

	t.Run("cluster reads Config via the fleet group", func(t *testing.T) {
		allowed, err := HasAccess(ctx, "Config", "any-config-uid", configRead)
		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("cluster cannot write Config", func(t *testing.T) {
		allowed, err := HasAccess(ctx, "Config", "any-config-uid", configWrite)
		require.NoError(t, err)
		assert.False(t, allowed)
	})
}
