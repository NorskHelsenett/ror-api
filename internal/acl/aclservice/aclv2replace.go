package aclservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
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
