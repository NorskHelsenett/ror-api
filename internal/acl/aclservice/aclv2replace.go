package aclservice

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"go.opentelemetry.io/otel/attribute"
)

func GetAllACL2(ctx context.Context) ([]aclmodels.AclV2ListItem, error) {
	all, err := aclStore.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	return aclmodels.V3ListToV2List(all), nil
}

// GetAccessByScopeSubject returns the caller's own ACL grants grouped by
// scope+subject, sourced from the V3 snapshot. Identity-less/cluster callers get
// an empty response.
func GetAccessByScopeSubject(ctx context.Context, scope aclmodels.Acl2Scope, subject aclmodels.Acl2Subject) (aclmodels.AclLookupResponse, error) {
	groups, err := identityGroups(ctx)
	if err != nil {
		return aclmodels.AclLookupResponse{}, nil // no groups -> no grants
	}

	entries, err := aclStore.GetByGroups(ctx, groups)
	if err != nil {
		return aclmodels.AclLookupResponse{}, fmt.Errorf("failed to load acl entries: %w", err)
	}

	return entries.FilterScopeSubject(scope.ToKind(), subject.ToKind()).ToV2LookupResponse(), nil
}

// GetAccessGroupsByScopeSubject returns every group's ACL grants that apply to
// the resource {scope, subject} — grants defined directly on it and grants
// inherited from an ancestor. Grants for the same (group, access) are merged
// into a single row, with Provenance listing every tree level (0 = the resource
// itself, 1 = its immediate parent, ...) the grant was found at. Global and
// ror-level scope grants are excluded (they have no resource provenance). This
// is an inspection view; the caller must gate it with HasAccess before calling.
func GetAccessGroupsByScopeSubject(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) ([]aclmodels.Acl3LookupByScopeSubjectAccessGroup, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.GetAccessGroupsByScopeSubject")
	defer span.End()
	span.SetAttributes(
		attribute.String("acl.scope", string(scope)),
		attribute.String("acl.subject", string(subject)),
	)

	// Relevant provenance keys, keyed by tree level: 0 is the resource itself,
	// then its ancestors nearest-first.
	levels := map[acl.Ownerref]int{
		{Scope: scope.ToKind(), Subject: subject}: 0,
	}
	if ancestorResolver != nil {
		ancestors, err := ancestorResolver.Ancestors(ctx, scope.ToKind(), subject)
		if err != nil {
			return nil, rortracer.SpanError(span, fmt.Errorf("failed to resolve ancestors: %w", err))
		}
		span.SetAttributes(attribute.Int("acl.ancestors", len(ancestors)))
		for i, a := range ancestors {
			levels[acl.Ownerref{Scope: a.Scope.ToKind(), Subject: a.Subject}] = i + 1
		}
	}
	span.SetAttributes(attribute.Int("acl.relevant_keys", len(levels)))

	entries, err := aclStore.GetAll(ctx)
	if err != nil {
		return nil, rortracer.SpanError(span, fmt.Errorf("failed to load acl entries: %w", err))
	}

	type groupAccessKey struct {
		group  string
		access aclmodels.AccessTypeV3
	}
	provenanceByKey := make(map[groupAccessKey][]aclmodels.Acl3LookupProvenance)
	seenRef := make(map[groupAccessKey]map[acl.Ownerref]struct{})
	for _, e := range entries {
		ref := acl.Ownerref{Scope: e.Scope.ToKind(), Subject: e.Subject}
		level, ok := levels[ref]
		if !ok {
			continue
		}
		for _, a := range e.Access {
			key := groupAccessKey{group: e.Group, access: a}
			if seenRef[key] == nil {
				seenRef[key] = make(map[acl.Ownerref]struct{})
			}
			if _, dup := seenRef[key][ref]; dup {
				continue
			}
			seenRef[key][ref] = struct{}{}
			provenanceByKey[key] = append(provenanceByKey[key], aclmodels.Acl3LookupProvenance{
				Scope:   ref.Scope,
				Subject: ref.Subject,
				Level:   level,
			})
		}
	}

	result := make([]aclmodels.Acl3LookupByScopeSubjectAccessGroup, 0, len(provenanceByKey))
	for key, provenance := range provenanceByKey {
		slices.SortFunc(provenance, func(a, b aclmodels.Acl3LookupProvenance) int {
			return cmp.Compare(a.Level, b.Level)
		})
		result = append(result, aclmodels.Acl3LookupByScopeSubjectAccessGroup{
			Access:     key.access,
			Group:      key.group,
			Provenance: provenance,
		})
	}
	slices.SortFunc(result, func(a, b aclmodels.Acl3LookupByScopeSubjectAccessGroup) int {
		if c := cmp.Compare(a.Group, b.Group); c != 0 {
			return c
		}
		return cmp.Compare(string(a.Access), string(b.Access))
	})

	span.SetAttributes(attribute.Int("acl.access_groups", len(result)))
	return result, nil
}

func GetGroupsInUse(ctx context.Context, groups []string) ([]string, error) {
	acls, err := aclStore.GetByGroups(ctx, groups)
	if err != nil {
		return nil, err
	}

	// Collect distinct group names; the list can hold multiple entries per group.
	seen := make(map[string]struct{}, len(acls))
	groupsInUse := make([]string, 0, len(acls))
	for _, acl := range acls {
		if _, ok := seen[acl.Group]; ok {
			continue
		}
		seen[acl.Group] = struct{}{}
		groupsInUse = append(groupsInUse, acl.Group)
	}

	return groupsInUse, nil
}

func GetV2ById(ctx context.Context, id string) (*aclmodels.AclV2ListItem, error) {
	entry, err := aclStore.GetById(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("could not get object by Id from store: %v", err)
	}
	if entry == nil {
		return nil, nil
	}

	v2entry := aclmodels.V3ToV2(*entry)

	return &v2entry, nil
}

// defaultFilterLimit mirrors the legacy mongoHelper default page size.
const defaultFilterLimit = 25

// GetByFilter returns a page of ACL entries matching the filter, plus the total
// number of matching entries (before pagination). Default limit 25, filters
// combined with AND, and a default sort on "group" ascending. Ordering is
// deterministic (entry id tiebreaker) so pagination is stable across snapshot
// reloads.
func getByFilter(ctx context.Context, filter *apicontracts.Filter) ([]aclmodels.AclV2ListItem, int, error) {
	entries, err := aclStore.GetAll(ctx)
	if err != nil {
		return nil, 0, err
	}

	skip, limit := 0, defaultFilterLimit
	sortField, asc := "group", true
	if filter != nil {
		skip = filter.Skip
		if filter.Limit > 0 {
			limit = filter.Limit
		}
		entries = entries.Filter(func(e aclmodels.AclV3ListItem) bool {
			return matchesAllFilters(e, filter.Filters)
		})
		if s, ok := primarySort(filter.Sort); ok {
			sortField, asc = s.SortField, s.SortOrder == 1
		}
	}

	totalCount := len(entries)
	page := entries.Sorted(sortField, asc).Page(skip, limit)
	return aclmodels.V3ListToV2List(page), totalCount, nil
}

func matchesAllFilters(e aclmodels.AclV3ListItem, filters []apicontracts.FilterMetadata) bool {
	for _, f := range filters {
		if !matchesFilter(e, f) {
			return false
		}
	}
	return true
}

func matchesFilter(e aclmodels.AclV3ListItem, f apicontracts.FilterMetadata) bool {
	if !validFilter(f) {
		return true // invalid filters impose no restriction
	}
	field := e.FieldValue(f.Field)
	switch f.MatchMode {
	case apicontracts.MatchModeContains:
		return strings.Contains(strings.ToLower(field), strings.ToLower(fmt.Sprint(f.Value)))
	case apicontracts.MatchModeEquals:
		return field == fmt.Sprint(f.Value)
	case apicontracts.MatchModeIn:
		vals, _ := f.Value.([]any)
		for _, v := range vals {
			if field == fmt.Sprint(v) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// validFilter mirrors mongoHelper.validFilter: only well-formed filters restrict.
func validFilter(f apicontracts.FilterMetadata) bool {
	switch f.MatchMode {
	case apicontracts.MatchModeContains, apicontracts.MatchModeEquals:
		return f.Value != nil && f.Value != "" && f.Field != ""
	case apicontracts.MatchModeIn:
		vals, ok := f.Value.([]any)
		return ok && len(vals) > 0
	default:
		return false
	}
}

// primarySort returns the first usable sort directive, if any.
func primarySort(sorts []apicontracts.SortMetadata) (apicontracts.SortMetadata, bool) {
	for _, s := range sorts {
		if s.SortField != "" && (s.SortOrder == 1 || s.SortOrder == -1) {
			return s, true
		}
	}
	return apicontracts.SortMetadata{}, false
}
