package aclservice

import (
	"context"
	"slices"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// fakeStore is an in-memory acl.GroupReader returning the entries whose Group
// is in the requested set.
type fakeStore struct{ entries aclmodels.AclV3List }

func (f fakeStore) GetByGroups(_ context.Context, groups []string) (aclmodels.AclV3List, error) {
	out := aclmodels.AclV3List{}
	for _, e := range f.entries {
		if slices.Contains(groups, e.Group) {
			out = append(out, e)
		}
	}
	return out, nil
}

// fakeExpander is an in-memory acl.ScopeExpander mapping a seed ownerref to its
// descendant ownerrefs.
type fakeExpander struct {
	descendants map[acl.Ownerref][]acl.Ownerref
}

func (f fakeExpander) ExpandScope(_ context.Context, scope aclscope.Scope, subject aclscope.Subject) ([]acl.Ownerref, error) {
	return f.descendants[acl.Ownerref{Scope: scope, Subject: subject}], nil
}

func (f fakeExpander) ExpandScopes(_ context.Context, seeds []acl.Ownerref) (map[acl.Ownerref][]acl.Ownerref, error) {
	out := make(map[acl.Ownerref][]acl.Ownerref, len(seeds))
	for _, s := range seeds {
		out[s] = f.descendants[s]
	}
	return out, nil
}

// setResolver swaps the package-level resolver for a fake-store-backed one and
// restores the previous resolver on cleanup. When an expander is supplied,
// HasAccess and ResourceOwnerFilter both resolve inherited (hierarchical)
// access, so they authorize the same ownerrefs.
func setResolver(t *testing.T, entries aclmodels.AclV3List, expander acl.ScopeExpander) {
	t.Helper()
	prev := resolver
	if expander != nil {
		resolver = acl.NewResolver(fakeStore{entries: entries}, acl.WithScopeExpander(expander))
	} else {
		resolver = acl.NewResolver(fakeStore{entries: entries})
	}
	t.Cleanup(func() { resolver = prev })
}

func userContext(groups ...string) context.Context {
	id := identitymodels.Identity{
		Type: identitymodels.IdentityTypeUser,
		User: &identitymodels.User{Email: "user@example.com", Groups: groups},
	}
	return context.WithValue(context.Background(), identitymodels.ContexIdentity, id)
}

// ownerFilterMatches evaluates the $match stage produced by ResourceOwnerFilter
// against a single ownerref, mirroring what MongoDB would do. It understands the
// shapes emitted by aclstore.OwnerrefsToFilter: empty (unrestricted), a single
// scope+subject match (deny-all), or an $or of scope-level and $in clauses.
func ownerFilterMatches(t *testing.T, filter bson.M, ref acl.Ownerref) bool {
	t.Helper()

	// Empty filter -> unrestricted, matches everything.
	if len(filter) == 0 {
		return true
	}

	match, ok := filter["$match"].(bson.M)
	require.True(t, ok, "filter must contain a $match stage: %v", filter)

	// Single scope+subject match (deny-all / cluster-identity shape).
	if scope, ok := match["rormeta.ownerref.scope"]; ok {
		return scope == string(ref.Scope) && match["rormeta.ownerref.subject"] == string(ref.Subject)
	}

	or, ok := match["$or"].(bson.A)
	require.True(t, ok, "expected $or in $match: %v", match)

	for _, clauseAny := range or {
		clause, ok := clauseAny.(bson.M)
		require.True(t, ok)

		// Scope-level grant: matches any subject within the scope.
		if scope, ok := clause["rormeta.ownerref.scope"]; ok {
			if scope == string(ref.Scope) {
				return true
			}
			continue
		}

		// Specific scope+subject pairs via $in.
		ownerref, ok := clause["rormeta.ownerref"].(bson.M)
		require.True(t, ok)
		inList, ok := ownerref["$in"].(bson.A)
		require.True(t, ok)
		for _, itemAny := range inList {
			item, ok := itemAny.(bson.D)
			require.True(t, ok)
			var scope, subject any
			for _, e := range item {
				switch e.Key {
				case "scope":
					scope = e.Value
				case "subject":
					subject = e.Value
				}
			}
			if scope == string(ref.Scope) && subject == string(ref.Subject) {
				return true
			}
		}
	}
	return false
}

func aclEntry(group string, scope aclscope.Scope, subject aclscope.Subject, access ...aclmodels.AccessTypeV3) aclmodels.AclV3ListItem {
	return aclmodels.AclV3ListItem{
		Version: 3,
		Group:   group,
		Scope:   scope,
		Subject: subject,
		Access:  access,
	}
}

// TestHasAccessAndResourceOwnerFilter asserts, for a range of ACL setups, the
// concrete authorization outcome (allowed / denied) for every candidate
// ownerref — verified independently through both HasAccess (the point check)
// and ResourceOwnerFilter (the query $match). Both must match the expected
// value, which also proves the two paths always agree.
func TestHasAccessAndResourceOwnerFilter(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	update := aclmodels.CapRor.WithVerb(aclmodels.VerbUpdate)

	// Candidate universe of ownerrefs, probed from both directions.
	var (
		clusterA = acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: "cluster-a"}
		clusterB = acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: "cluster-b"}
		proj1    = acl.Ownerref{Scope: aclscope.ScopeProject, Subject: "proj-1"}
		proj2    = acl.Ownerref{Scope: aclscope.ScopeProject, Subject: "proj-2"}
		dc1      = acl.Ownerref{Scope: aclscope.ScopeDatacenter, Subject: "dc-1"}
		vm1      = acl.Ownerref{Scope: aclscope.ScopeVirtualMachine, Subject: "vm-1"}
	)
	universe := []acl.Ownerref{clusterA, clusterB, proj1, proj2, dc1, vm1}

	tests := []struct {
		name     string
		groups   []string // defaults to {"dev-team"} when empty
		entries  aclmodels.AclV3List
		expander acl.ScopeExpander
		allowed  []acl.Ownerref // every other ownerref in the universe must be denied
	}{
		{
			name: "specific grants",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeCluster, "cluster-a", read),
				aclEntry("dev-team", aclscope.ScopeProject, "proj-1", read),
			},
			allowed: []acl.Ownerref{clusterA, proj1},
		},
		{
			name: "scope-level (ror) grant on clusters",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeRor, aclscope.Subject(aclscope.ScopeCluster), read),
			},
			allowed: []acl.Ownerref{clusterA, clusterB},
		},
		{
			name: "scope-level (ror) grant on projects",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeRor, aclscope.Subject(aclscope.ScopeProject), read),
			},
			allowed: []acl.Ownerref{proj1, proj2},
		},
		{
			name: "global grant (ror/globalscope)",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeRor, aclscope.SubjectGlobal, read),
			},
			allowed: universe,
		},
		{
			name: "global grant (scope=all)",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeAll, "anything", read),
			},
			allowed: universe,
		},
		{
			name: "global grant (subject=all)",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeCluster, aclscope.SubjectAll, read),
			},
			allowed: universe,
		},
		{
			name: "grant for another access type only",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeCluster, "cluster-a", update),
			},
			allowed: nil,
		},
		{
			name: "inherited grant via scope expander",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeProject, "proj-1", read),
			},
			expander: fakeExpander{descendants: map[acl.Ownerref][]acl.Ownerref{
				proj1: {clusterA, dc1},
			}},
			allowed: []acl.Ownerref{proj1, clusterA, dc1},
		},
		{
			name: "datacenter-level grant cascades through expander",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeDatacenter, "dc-1", read),
			},
			expander: fakeExpander{descendants: map[acl.Ownerref][]acl.Ownerref{
				dc1: {proj1, clusterA, clusterB},
			}},
			allowed: []acl.Ownerref{dc1, proj1, clusterA, clusterB},
		},
		{
			name: "expander maps a non-granted seed only (no expansion)",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeProject, "proj-2", read),
			},
			expander: fakeExpander{descendants: map[acl.Ownerref][]acl.Ownerref{
				proj1: {clusterA, dc1}, // proj-1 is never granted, so never a seed
			}},
			allowed: []acl.Ownerref{proj2},
		},
		{
			name: "expansion respects required access type",
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeProject, "proj-1", update), // update, not read
			},
			expander: fakeExpander{descendants: map[acl.Ownerref][]acl.Ownerref{
				proj1: {clusterA, dc1},
			}},
			allowed: nil,
		},
		{
			name:   "grants aggregated across multiple groups",
			groups: []string{"dev-team", "ops-team"},
			entries: aclmodels.AclV3List{
				aclEntry("dev-team", aclscope.ScopeCluster, "cluster-a", read),
				aclEntry("ops-team", aclscope.ScopeProject, "proj-1", read),
			},
			allowed: []acl.Ownerref{clusterA, proj1},
		},
		{
			name: "grant for a group the caller is not in",
			entries: aclmodels.AclV3List{
				aclEntry("other-team", aclscope.ScopeCluster, "cluster-a", read),
			},
			allowed: nil,
		},
		{
			name:    "no grants (deny all)",
			entries: aclmodels.AclV3List{},
			allowed: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setResolver(t, tc.entries, tc.expander)

			groups := tc.groups
			if len(groups) == 0 {
				groups = []string{"dev-team"}
			}
			ctx := userContext(groups...)

			allowedSet := make(map[acl.Ownerref]bool, len(tc.allowed))
			for _, ref := range tc.allowed {
				allowedSet[ref] = true
			}

			filter, err := ResourceOwnerFilter(ctx, read)
			require.NoError(t, err)

			for _, ref := range universe {
				want := allowedSet[ref]

				gotHasAccess, err := HasAccess(ctx, ref.Scope, ref.Subject, read)
				require.NoError(t, err)
				assert.Equalf(t, want, gotHasAccess,
					"HasAccess(%s/%s): want %v", ref.Scope, ref.Subject, want)

				gotFilter := ownerFilterMatches(t, filter, ref)
				assert.Equalf(t, want, gotFilter,
					"ResourceOwnerFilter match(%s/%s): want %v", ref.Scope, ref.Subject, want)
			}
		})
	}
}
