package aclservice

import (
	"testing"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubClusterResolver installs a ClusterIdToUidResolver backed by the given
// name->uid map and restores the previous resolver on cleanup.
func stubClusterResolver(t *testing.T, mapping map[string]string) {
	t.Helper()
	prev := aclmodels.ClusterIdToUidResolver
	aclmodels.ClusterIdToUidResolver = func(clusterID string) string { return mapping[clusterID] }
	t.Cleanup(func() { aclmodels.ClusterIdToUidResolver = prev })
}

// TestHasAccessResolvesClusterIdToUid verifies that cluster-scoped checks
// resolve a human-readable cluster id to its uid (the key ACL entries use)
// before consulting the store, while uid subjects and non-cluster scopes are
// passed through unchanged.
func TestHasAccessResolvesClusterIdToUid(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	const (
		clusterName = "prod-cluster"
		clusterUID  = "11111111-1111-1111-1111-111111111111"
	)

	// ACL grants read only on the cluster uid.
	entries := aclmodels.AclV3List{
		aclEntry("dev-team", aclscope.ScopeCluster, aclscope.Subject(clusterUID), read),
	}

	t.Run("cluster id is resolved to uid", func(t *testing.T) {
		setResolver(t, entries, nil)
		stubClusterResolver(t, map[string]string{clusterName: clusterUID})
		ctx := userContext("dev-team")

		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, clusterName, read)
		require.NoError(t, err)
		assert.True(t, allowed, "cluster id should resolve to uid and be granted")
	})

	t.Run("uid subject passes through unchanged", func(t *testing.T) {
		setResolver(t, entries, nil)
		// Resolver maps the name elsewhere; a uid subject must not be looked up.
		stubClusterResolver(t, map[string]string{clusterName: "should-not-be-used"})
		ctx := userContext("dev-team")

		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, aclscope.Subject(clusterUID), read)
		require.NoError(t, err)
		assert.True(t, allowed, "uid subject should be granted directly")
	})

	t.Run("unknown cluster id is denied", func(t *testing.T) {
		setResolver(t, entries, nil)
		stubClusterResolver(t, map[string]string{clusterName: clusterUID})
		ctx := userContext("dev-team")

		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, "unknown-cluster", read)
		require.NoError(t, err)
		assert.False(t, allowed, "unresolvable cluster id should be denied")
	})

	t.Run("no resolver wired leaves subject unchanged", func(t *testing.T) {
		setResolver(t, entries, nil)
		prev := aclmodels.ClusterIdToUidResolver
		aclmodels.ClusterIdToUidResolver = nil
		t.Cleanup(func() { aclmodels.ClusterIdToUidResolver = prev })
		ctx := userContext("dev-team")

		allowed, err := HasAccess(ctx, aclscope.ScopeCluster, clusterName, read)
		require.NoError(t, err)
		assert.False(t, allowed, "without a resolver the cluster id cannot be matched")
	})

	t.Run("non-cluster scope is not resolved", func(t *testing.T) {
		// A project grant keyed by the same string; resolver would rewrite it if
		// (incorrectly) applied to non-cluster scopes.
		projectEntries := aclmodels.AclV3List{
			aclEntry("dev-team", aclscope.ScopeProject, aclscope.Subject(clusterName), read),
		}
		setResolver(t, projectEntries, nil)
		stubClusterResolver(t, map[string]string{clusterName: clusterUID})
		ctx := userContext("dev-team")

		allowed, err := HasAccess(ctx, aclscope.ScopeProject, aclscope.Subject(clusterName), read)
		require.NoError(t, err)
		assert.True(t, allowed, "project subject must be used verbatim, not resolved")
	})
}

// TestResolveOwnerrefsResolvesClusterFilterSubjects verifies that a cluster-id
// subject in the ResolveOwnerrefs filter is resolved to its uid so it matches
// the uid-keyed store, while non-cluster filters are left untouched.
func TestResolveOwnerrefsResolvesClusterFilterSubjects(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	const (
		clusterName = "prod-cluster"
		clusterUID  = "33333333-3333-3333-3333-333333333333"
	)

	entries := aclmodels.AclV3List{
		aclEntry("dev-team", aclscope.ScopeCluster, aclscope.Subject(clusterUID), read),
	}

	t.Run("cluster-id filter subject resolves to uid", func(t *testing.T) {
		setResolver(t, entries, nil)
		stubClusterResolver(t, map[string]string{clusterName: clusterUID})
		ctx := userContext("dev-team")

		refs, unrestricted, err := ResolveOwnerrefs(ctx, read, acl.OwnerrefFilter{
			Scopes:   []aclscope.Scope{aclscope.ScopeCluster},
			Subjects: []aclscope.Subject{clusterName},
		})
		require.NoError(t, err)
		assert.False(t, unrestricted)
		assert.Contains(t, refs, acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: aclscope.Subject(clusterUID)})
	})

	t.Run("uid filter subject still matches", func(t *testing.T) {
		setResolver(t, entries, nil)
		stubClusterResolver(t, map[string]string{clusterName: clusterUID})
		ctx := userContext("dev-team")

		refs, _, err := ResolveOwnerrefs(ctx, read, acl.OwnerrefFilter{
			Scopes:   []aclscope.Scope{aclscope.ScopeCluster},
			Subjects: []aclscope.Subject{clusterUID},
		})
		require.NoError(t, err)
		assert.Contains(t, refs, acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: aclscope.Subject(clusterUID)})
	})

	t.Run("non-cluster scope filter is not resolved", func(t *testing.T) {
		projectEntries := aclmodels.AclV3List{
			aclEntry("dev-team", aclscope.ScopeProject, aclscope.Subject(clusterName), read),
		}
		setResolver(t, projectEntries, nil)
		stubClusterResolver(t, map[string]string{clusterName: clusterUID})
		ctx := userContext("dev-team")

		refs, _, err := ResolveOwnerrefs(ctx, read, acl.OwnerrefFilter{
			Scopes:   []aclscope.Scope{aclscope.ScopeProject},
			Subjects: []aclscope.Subject{clusterName},
		})
		require.NoError(t, err)
		assert.Contains(t, refs, acl.Ownerref{Scope: aclscope.ScopeProject, Subject: aclscope.Subject(clusterName)})
	})
}
