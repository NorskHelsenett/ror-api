package aclservice

import (
	"testing"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// matchStage extracts the inner bson.M of a {"$match": {...}} pipeline stage.
func matchStage(t *testing.T, filter bson.M) bson.M {
	t.Helper()
	m, ok := filter["$match"].(bson.M)
	require.True(t, ok, "filter must contain a $match stage: %v", filter)
	return m
}

// TestClusterUIDFilter asserts the $match stage ClusterUIDFilter produces for
// the clusters collection (keyed by top-level uid) across the authorization
// shapes the legacy GetACL2ByIdentityQuery + CreateClusterACLFilter pair covered.
func TestClusterUIDFilter(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)

	t.Run("direct cluster grants -> sorted uid $in", func(t *testing.T) {
		entries := aclmodels.AclV3List{
			aclEntry("dev-team", aclscope.ScopeCluster, "uid-b", read),
			aclEntry("dev-team", aclscope.ScopeCluster, "uid-a", read),
		}
		setResolver(t, entries, nil)
		ctx := userContext("dev-team")

		filter, err := ClusterUIDFilter(ctx, read)
		require.NoError(t, err)
		assert.Equal(t, bson.M{"uid": bson.M{"$in": []string{"uid-a", "uid-b"}}}, matchStage(t, filter))
	})

	t.Run("ror-level all-clusters grant -> match all", func(t *testing.T) {
		entries := aclmodels.AclV3List{
			aclEntry("admins", aclscope.ScopeRor, aclscope.Subject(aclscope.ScopeCluster), read),
		}
		setResolver(t, entries, nil)
		ctx := userContext("admins")

		filter, err := ClusterUIDFilter(ctx, read)
		require.NoError(t, err)
		assert.Empty(t, matchStage(t, filter), "a ror-level cluster grant must see every cluster")
	})

	t.Run("global grant -> match all", func(t *testing.T) {
		entries := aclmodels.AclV3List{
			aclEntry("super", aclscope.ScopeAll, aclscope.SubjectAll, read),
		}
		setResolver(t, entries, nil)
		ctx := userContext("super")

		filter, err := ClusterUIDFilter(ctx, read)
		require.NoError(t, err)
		assert.Empty(t, matchStage(t, filter))
	})

	t.Run("no matching access -> deny all", func(t *testing.T) {
		entries := aclmodels.AclV3List{
			aclEntry("other-team", aclscope.ScopeCluster, "uid-x", read),
		}
		setResolver(t, entries, nil)
		ctx := userContext("dev-team") // no entries for this group

		filter, err := ClusterUIDFilter(ctx, read)
		require.NoError(t, err)
		assert.Equal(t, bson.M{"uid": bson.M{"$in": bson.A{"Unknown-Unauthorized"}}}, matchStage(t, filter))
	})

	t.Run("cluster identity -> own uid", func(t *testing.T) {
		setResolver(t, aclmodels.AclV3List{}, nil)
		ctx := clusterContext("prod-cluster", "cluster-uid-1")

		filter, err := ClusterUIDFilter(ctx, read)
		require.NoError(t, err)
		assert.Equal(t, bson.M{"uid": bson.M{"$in": []string{"cluster-uid-1"}}}, matchStage(t, filter))
	})

	t.Run("inherited clusters via parent-scope expansion", func(t *testing.T) {
		entries := aclmodels.AclV3List{
			aclEntry("dev-team", aclscope.ScopeProject, "proj-1", read),
		}
		expander := fakeExpander{descendants: map[acl.Ownerref][]acl.Ownerref{
			{Scope: aclscope.ScopeProject, Subject: "proj-1"}: {
				{Scope: aclscope.ScopeCluster, Subject: "uid-child-a"},
				{Scope: aclscope.ScopeCluster, Subject: "uid-child-b"},
			},
		}}
		setResolver(t, entries, expander)
		ctx := userContext("dev-team")

		filter, err := ClusterUIDFilter(ctx, read)
		require.NoError(t, err)
		assert.Equal(t, bson.M{"uid": bson.M{"$in": []string{"uid-child-a", "uid-child-b"}}}, matchStage(t, filter))
	})
}
