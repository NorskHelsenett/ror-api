package aclservice

import (
	"context"
	"fmt"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/acl/aclstore"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// resolver is the package-level ACL resolver.
// Must be initialized by calling InitResolver before use.
var resolver *acl.Resolver

// InitResolver initializes the ACL resolver backed by MongoDB.
// Call this during application startup after MongoDB is initialized.
func InitResolver() {
	db := mongodb.GetMongoDb()
	store := aclstore.NewMongoStore(db)
	expander := newMongoScopeExpander(db)
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

	refs, err := resolver.ResolveOwnerrefs(ctx, groups, required)
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
