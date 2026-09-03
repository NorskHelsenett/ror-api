package aclservice

import (
	"context"
	"slices"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/acl"
	aclstorev2 "github.com/NorskHelsenett/ror/pkg/acl/aclstore/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAclStore implements aclstorev2.Store over a fixed entry list. Only the
// read methods exercised by the lookup are meaningful; the rest are stubs.
type fakeAclStore struct{ entries aclmodels.AclV3List }

var _ aclstorev2.Store = fakeAclStore{}

func (f fakeAclStore) GetAll(_ context.Context) (aclmodels.AclV3List, error) { return f.entries, nil }

func (f fakeAclStore) GetByGroups(_ context.Context, groups []string) (aclmodels.AclV3List, error) {
	out := aclmodels.AclV3List{}
	for _, e := range f.entries {
		if slices.Contains(groups, e.Group) {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f fakeAclStore) GetByScopeSubject(_ context.Context, _ aclscope.Scope, _ aclscope.Subject) (aclmodels.AclV3List, error) {
	return nil, nil
}
func (f fakeAclStore) GetById(_ context.Context, _ string) (*aclmodels.AclV3ListItem, error) {
	return nil, nil
}
func (f fakeAclStore) Create(_ context.Context, item aclmodels.AclV3ListItem) (*aclmodels.AclV3ListItem, error) {
	return &item, nil
}
func (f fakeAclStore) Update(_ context.Context, _ string, item aclmodels.AclV3ListItem) (*aclmodels.AclV3ListItem, *aclmodels.AclV3ListItem, error) {
	return &item, nil, nil
}
func (f fakeAclStore) Delete(_ context.Context, _ string) (*aclmodels.AclV3ListItem, error) {
	return nil, nil
}

// fakeAncestors implements acl.AncestorResolver from a static map.
type fakeAncestors struct {
	byResource map[acl.Ownerref][]acl.Ownerref
}

func (f fakeAncestors) Ancestors(_ context.Context, scope aclscope.Scope, subject aclscope.Subject) ([]acl.Ownerref, error) {
	return f.byResource[acl.Ownerref{Scope: scope, Subject: subject}], nil
}

func setAclStore(t *testing.T, s aclstorev2.Store) {
	t.Helper()
	prev := aclStore
	aclStore = s
	t.Cleanup(func() { aclStore = prev })
}

func setAncestorResolver(t *testing.T, r acl.AncestorResolver) {
	t.Helper()
	prev := ancestorResolver
	ancestorResolver = r
	t.Cleanup(func() { ancestorResolver = prev })
}

// TestGetAccessGroupsByScopeSubject asserts that the lookup returns every
// group's grants defined on the resource or an ancestor, merged into one row
// per (group, access) with Provenance listing each matching tree level, and
// excludes unrelated resources, global grants, and ror-level scope grants.
func TestGetAccessGroupsByScopeSubject(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	update := aclmodels.CapRor.WithVerb(aclmodels.VerbUpdate)

	const clusterUID = "cluster-uid-1"
	const projUID = "proj-1"
	const dcUID = "dc-1"

	ancestors := fakeAncestors{byResource: map[acl.Ownerref][]acl.Ownerref{
		{Scope: aclscope.ScopeCluster, Subject: clusterUID}: {
			{Scope: aclscope.ScopeProject, Subject: projUID},  // level 1
			{Scope: aclscope.ScopeDatacenter, Subject: dcUID}, // level 2
		},
	}}

	entries := aclmodels.AclV3List{
		aclEntry("team-direct", aclscope.ScopeCluster, clusterUID, read),                             // direct on resource (level 0)
		aclEntry("team-proj", aclscope.ScopeProject, projUID, read, update),                          // inherited (level 1)
		aclEntry("team-dc", aclscope.ScopeDatacenter, dcUID, read),                                   // inherited (level 2)
		aclEntry("team-other", aclscope.ScopeCluster, "other-cluster", read),                         // unrelated resource
		aclEntry("admins", aclscope.ScopeRor, aclscope.SubjectGlobal, read),                          // global -> excluded
		aclEntry("cluster-admins", aclscope.ScopeRor, aclscope.Subject(aclscope.ScopeCluster), read), // ror-level -> excluded
	}

	setAclStore(t, fakeAclStore{entries: entries})
	setAncestorResolver(t, ancestors)

	got, err := GetAccessGroupsByScopeSubject(context.Background(), aclscope.ScopeCluster, clusterUID)
	require.NoError(t, err)

	// Deterministically sorted by Group then Access.
	want := []aclmodels.Acl3LookupByScopeSubjectAccessGroup{
		{Access: read, Group: "team-dc", Provenance: []aclmodels.Acl3LookupProvenance{
			{Scope: aclscope.ScopeDatacenter, Subject: dcUID, Level: 2},
		}},
		{Access: read, Group: "team-direct", Provenance: []aclmodels.Acl3LookupProvenance{
			{Scope: aclscope.ScopeCluster, Subject: clusterUID, Level: 0},
		}},
		{Access: read, Group: "team-proj", Provenance: []aclmodels.Acl3LookupProvenance{
			{Scope: aclscope.ScopeProject, Subject: projUID, Level: 1},
		}},
		{Access: update, Group: "team-proj", Provenance: []aclmodels.Acl3LookupProvenance{
			{Scope: aclscope.ScopeProject, Subject: projUID, Level: 1},
		}},
	}
	assert.Equal(t, want, got)
}

// TestGetAccessGroupsByScopeSubject_MergesMultipleLevels asserts that a group
// holding the same access at more than one tree level (e.g. directly on the
// resource and again on an ancestor) gets a single row whose Provenance lists
// every level, ordered nearest-first.
func TestGetAccessGroupsByScopeSubject_MergesMultipleLevels(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	const clusterUID = "cluster-uid-1"
	const projUID = "proj-1"

	ancestors := fakeAncestors{byResource: map[acl.Ownerref][]acl.Ownerref{
		{Scope: aclscope.ScopeCluster, Subject: clusterUID}: {
			{Scope: aclscope.ScopeProject, Subject: projUID},
		},
	}}

	entries := aclmodels.AclV3List{
		aclEntry("team-both", aclscope.ScopeCluster, clusterUID, read), // level 0
		aclEntry("team-both", aclscope.ScopeProject, projUID, read),    // level 1
	}
	setAclStore(t, fakeAclStore{entries: entries})
	setAncestorResolver(t, ancestors)

	got, err := GetAccessGroupsByScopeSubject(context.Background(), aclscope.ScopeCluster, clusterUID)
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.Equal(t, aclmodels.Acl3LookupByScopeSubjectAccessGroup{
		Access: read,
		Group:  "team-both",
		Provenance: []aclmodels.Acl3LookupProvenance{
			{Scope: aclscope.ScopeCluster, Subject: clusterUID, Level: 0},
			{Scope: aclscope.ScopeProject, Subject: projUID, Level: 1},
		},
	}, got[0])
}

// TestGetAccessGroupsByScopeSubject_Dedup ensures a duplicate ACL entry for the
// same (group, access, resource) does not produce a duplicate provenance entry.
func TestGetAccessGroupsByScopeSubject_Dedup(t *testing.T) {
	read := aclmodels.CapRor.WithVerb(aclmodels.VerbRead)
	const clusterUID = "cluster-uid-1"

	entries := aclmodels.AclV3List{
		aclEntry("team-a", aclscope.ScopeCluster, clusterUID, read),
		aclEntry("team-a", aclscope.ScopeCluster, clusterUID, read), // identical duplicate
	}
	setAclStore(t, fakeAclStore{entries: entries})
	setAncestorResolver(t, fakeAncestors{})

	got, err := GetAccessGroupsByScopeSubject(context.Background(), aclscope.ScopeCluster, clusterUID)
	require.NoError(t, err)
	assert.Equal(t, []aclmodels.Acl3LookupByScopeSubjectAccessGroup{
		{Access: read, Group: "team-a", Provenance: []aclmodels.Acl3LookupProvenance{
			{Scope: aclscope.ScopeCluster, Subject: clusterUID, Level: 0},
		}},
	}, got)
}
