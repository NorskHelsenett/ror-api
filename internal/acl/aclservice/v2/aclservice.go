package aclservice

import (
	"context"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/acl/aclstore"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/clients/redisdb"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// aclCacheTTL is the time-to-live for cached ACL entries and scope expansions.
const aclCacheTTL = 5 * time.Minute

// resolver is the package-level ACL resolver.
// Must be initialized by calling InitResolver before use.
var resolver *acl.Resolver

// InitResolver initializes the ACL resolver backed by MongoDB, fronted by a
// Redis-cached store and an in-memory cached scope expander for hierarchical
// (inherited) access resolution.
// Call this during application startup after MongoDB and Redis are initialized.
func InitResolver(redis redisdb.RedisDB) {
	// mongodb.GetMongoDb returns the live database handle on every call. It must
	// be passed as a provider (not invoked once here): the mongo client is
	// reconnected and the previous one disconnected on credential rotation, so a
	// captured handle would fail with "client is disconnected" after the first
	// renewal.
	store := acl.Store(aclstore.NewMongoStore(mongodb.GetMongoDb))
	if redis != nil {
		store = aclstore.NewCachedStore(store, redis, aclCacheTTL)
	}

	expander := acl.ScopeExpander(aclstore.NewMongoScopeExpander(mongodb.GetMongoDb))
	expander = acl.NewCachedScopeExpander(expander, aclCacheTTL)

	resolver = acl.NewResolver(store, acl.WithScopeExpander(expander))
}

// identityGroups extracts the group list from the context identity.
// For cluster identities, returns an error — callers must handle clusters separately.
func identityGroups(ctx context.Context) ([]string, error) {
	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity from context: %w", err)
	}

	if identity.IsCluster() {
		return nil, fmt.Errorf("cluster identities do not have groups")
	}

	if identity.IsUser() {
		if identity.User == nil {
			return nil, fmt.Errorf("user identity has nil user")
		}
		return identity.User.Groups, nil
	}

	if identity.IsService() {
		groups := []string{fmt.Sprintf("service-%s@ror.system", identity.GetId())}
		return groups, nil
	}

	return nil, fmt.Errorf("unknown identity type")
}

// HasAccess checks if the caller (from context) has the required access type
// for the given scope and subject.
//
// Cluster identities have implicit read/create/update access to their own resources
// (scope=cluster, subject=clusterID).
func HasAccess(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject, required aclmodels.AccessTypeV3) (bool, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.HasAccess")
	defer span.End()

	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get identity from context: %w", err)
	}

	// Cluster identities have implicit access to their own resources
	if identity.IsCluster() {
		if scope == aclscope.ScopeCluster && subject == aclscope.Subject(identity.GetId()) {
			return isImplicitClusterAccess(required), nil
		}
		return false, nil
	}

	groups, err := identityGroups(ctx)
	if err != nil {
		return false, err
	}

	return resolver.HasAccess(ctx, groups, scope, subject, required)
}

// ResolveAccess returns all access types the caller has for the given scope and subject.
func ResolveAccess(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) ([]aclmodels.AccessTypeV3, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResolveAccess")
	defer span.End()

	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity from context: %w", err)
	}

	if identity.IsCluster() {
		if scope == aclscope.ScopeCluster && subject == aclscope.Subject(identity.GetId()) {
			return implicitClusterAccessTypes(), nil
		}
		return nil, nil
	}

	groups, err := identityGroups(ctx)
	if err != nil {
		return nil, err
	}

	return resolver.ResolveAccess(ctx, groups, scope, subject)
}

// ResolveOwnerrefs returns the scope+subject pairs the caller has the required
// access type for. The unrestricted return value is true when the caller has
// global access for the required access type; in that case the returned slice
// is empty.
//
// The optional filter narrows the result to specific scopes and/or subjects.
//
// Cluster identities resolve to their own resource only.
func ResolveOwnerrefs(ctx context.Context, required aclmodels.AccessTypeV3, filter acl.OwnerrefFilter) (refs []acl.Ownerref, unrestricted bool, err error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResolveOwnerrefs")
	defer span.End()

	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get identity from context: %w", err)
	}

	if identity.IsCluster() {
		if isImplicitClusterAccess(required) {
			ref := acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: aclscope.Subject(identity.GetId())}
			if !filter.Matches(ref) {
				return []acl.Ownerref{}, false, nil
			}
			return []acl.Ownerref{ref}, false, nil
		}
		return nil, false, nil
	}

	groups, err := identityGroups(ctx)
	if err != nil {
		return nil, false, err
	}

	resolved, err := resolver.ResolveOwnerrefs(ctx, groups, required, filter)
	if err != nil {
		return nil, false, fmt.Errorf("failed to resolve ownerrefs: %w", err)
	}

	// The resolver returns a nil slice to signal unrestricted (global) access.
	if resolved == nil {
		return nil, true, nil
	}

	return resolved, false, nil
}

// ResourceOwnerFilter returns a MongoDB aggregation pipeline stage that scopes
// resource queries to resources the caller has the required access type for.
//
// For cluster identities, returns a filter matching only their own resources.
// For user/service identities, resolves ownerrefs via the resolver.
func ResourceOwnerFilter(ctx context.Context, required aclmodels.AccessTypeV3) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResourceOwnerFilter")
	defer span.End()

	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return aclstore.DenyAllFilter, fmt.Errorf("failed to get identity from context: %w", err)
	}

	if identity.IsCluster() {
		return aclstore.ClusterIdentityFilter(identity.GetId()), nil
	}

	groups, err := identityGroups(ctx)
	if err != nil {
		return aclstore.DenyAllFilter, err
	}

	refs, err := resolver.ResolveOwnerrefs(ctx, groups, required, acl.OwnerrefFilter{})
	if err != nil {
		return aclstore.DenyAllFilter, fmt.Errorf("failed to resolve ownerrefs: %w", err)
	}

	return aclstore.OwnerrefsToFilter(refs), nil
}

// ResourceTypeReadFilter returns a MongoDB aggregation pipeline stage that excludes
// resource kinds the caller is not authorized to read at the given scope and subject.
//
// Cluster identities get no type restriction (empty filter).
func ResourceTypeReadFilter(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResourceTypeReadFilter")
	defer span.End()

	access, err := ResolveAccess(ctx, scope, subject)
	if err != nil {
		return bson.M{}, err
	}

	return aclstore.ResourceTypeFilter(access), nil
}

// ResourceTypeWriteFilter returns a MongoDB aggregation pipeline stage that excludes
// resource kinds the caller is not authorized to write at the given scope and subject.
//
// Cluster identities get no type restriction (empty filter).
func ResourceTypeWriteFilter(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResourceTypeWriteFilter")
	defer span.End()

	access, err := ResolveAccess(ctx, scope, subject)
	if err != nil {
		return bson.M{}, err
	}

	return aclstore.ResourceTypeWriteFilter(access), nil
}

// isImplicitClusterAccess returns true if the given access type is one that
// clusters implicitly have for their own resources (read, create, update).
func isImplicitClusterAccess(access aclmodels.AccessTypeV3) bool {
	cap, verb := access.Parse()
	switch cap {
	case aclmodels.CapRor:
		return verb == aclmodels.VerbRead ||
			verb == aclmodels.VerbCreate ||
			verb == aclmodels.VerbUpdate
	case aclmodels.CapKubernetes:
		return verb == aclmodels.VerbRead ||
			verb == aclmodels.VerbCreate ||
			verb == aclmodels.VerbUpdate
	default:
		return false
	}
}

// implicitClusterAccessTypes returns the set of access types that clusters
// implicitly have for their own resources.
func implicitClusterAccessTypes() []aclmodels.AccessTypeV3 {
	return []aclmodels.AccessTypeV3{
		aclmodels.CapRor.WithVerb(aclmodels.VerbRead),
		aclmodels.CapRor.WithVerb(aclmodels.VerbCreate),
		aclmodels.CapRor.WithVerb(aclmodels.VerbUpdate),
		aclmodels.CapKubernetes.WithVerb(aclmodels.VerbRead),
		aclmodels.CapKubernetes.WithVerb(aclmodels.VerbCreate),
		aclmodels.CapKubernetes.WithVerb(aclmodels.VerbUpdate),
	}
}
