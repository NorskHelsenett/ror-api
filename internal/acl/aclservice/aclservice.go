package aclservice

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
	"uuid"

	apiaclstore "github.com/NorskHelsenett/ror-api/internal/acl/aclstore"
	"github.com/NorskHelsenett/ror-api/internal/auditlog"
	"github.com/NorskHelsenett/ror-api/internal/models"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/acl/aclevents"
	"github.com/NorskHelsenett/ror/pkg/acl/aclstore"
	aclstorev2 "github.com/NorskHelsenett/ror/pkg/acl/aclstore/v2"
	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/clients/rabbitmqclient"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
)

// aclCacheTTL is the time-to-live for cached scope expansions.
const aclCacheTTL = 5 * time.Minute

// defaultOwnerScopeCacheTTL is the fallback owner-uid cache TTL for the Mongo
// scope expander when ACL_SCOPE_OWNER_CACHE_TTL is unset or invalid.
const defaultOwnerScopeCacheTTL = 30 * time.Second

// ownerScopeCacheTTL reads the scope-expander owner-uid cache TTL from
// ACL_SCOPE_OWNER_CACHE_TTL (a Go duration, e.g. "30s"), falling back to the
// default when unset or unparseable.
func ownerScopeCacheTTL() time.Duration {
	raw := rorconfig.GetString("ACL_SCOPE_OWNER_CACHE_TTL")
	if raw == "" {
		return defaultOwnerScopeCacheTTL
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		rlog.Warn("invalid ACL_SCOPE_OWNER_CACHE_TTL, using default", rlog.String("value", raw))
		return defaultOwnerScopeCacheTTL
	}
	return ttl
}

// Package-level ACL state. Must be initialized by calling InitResolver.
var (
	resolver         *acl.Resolver
	aclStore         aclstorev2.Store
	refresher        *aclstorev2.Refresher
	ancestorResolver acl.AncestorResolver
)

// InitResolver initializes the ACL resolver backed by an in-memory snapshot of
// the ACL store (rebuilt from MongoDB) plus an in-memory cached scope expander
// for hierarchical (inherited) access resolution. The snapshot is loaded
// synchronously, so a failure is returned to let the caller gate startup; it is
// then kept fresh by a periodic reload and by change signals over rabbitmq.
// Call this during application startup after MongoDB and RabbitMQ are ready. A
// nil rmq connection disables the change bus (freshness then relies on the
// periodic reload only).
func InitResolver(rmq rabbitmqclient.RabbitMQConnection) error {
	// mongodb.GetMongoDb returns the live database handle on every call. It must
	// be passed as a provider (not invoked once here): the mongo client is
	// reconnected and the previous one disconnected on credential rotation, so a
	// captured handle would fail with "client is disconnected" after the first
	// renewal.
	mongoStore := apiaclstore.NewMongoStore(mongodb.GetMongoDb)
	snapshot := aclstorev2.NewSnapshotStore(mongoStore)

	// A background context is used so the reload loop lives for the process
	// lifetime rather than being cancelled with a startup context.
	refresher = aclstorev2.NewRefresher(snapshot)
	if err := refresher.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to load initial ACL snapshot: %w", err)
	}

	// Change bus: subscribe so writes on other instances refresh this snapshot,
	// and publish so this instance's writes refresh the others. Without a
	// connection, cross-instance freshness relies on the periodic reload above.
	var publisher aclstorev2.ChangePublisher = aclstorev2.NoopPublisher{}
	if rmq != nil {
		subscriber := aclevents.NewSubscriber(rmq, aclevents.DefaultExchangeName, aclevents.DefaultRoutingKey, refresher.Notify)
		if err := subscriber.Start(); err != nil {
			return fmt.Errorf("failed to start ACL change subscriber: %w", err)
		}
		publisher = aclevents.NewPublisher(rmq, aclevents.DefaultRoutingKey)
	}
	aclStore = aclstorev2.NewNotifyingStore(snapshot, publisher)

	expander := acl.ScopeExpander(aclstore.NewMongoScopeExpander(mongodb.GetMongoDb, aclstore.WithOwnerUidsTTL(ownerScopeCacheTTL())))
	expander = acl.NewCachedScopeExpander(expander, aclCacheTTL)

	resolver = acl.NewResolver(snapshot, acl.WithScopeExpander(expander))

	ancestorResolver = aclstore.NewMongoAncestorResolver(mongodb.GetMongoDb)
	return nil
}

// Store returns the ACL store to use for writes. Writes made through it are
// broadcast (once the change bus is wired) so every instance refreshes.
func Store() aclstorev2.Store { return aclStore }

// SetResolver replaces the package-level resolver. InitResolver is the normal
// entry point; this exists for tests and callers that supply their own ACL
// source without the MongoDB/RabbitMQ wiring.
func SetResolver(r *acl.Resolver) { resolver = r }

// errResolverNotInitialized is returned when ACL resolution is attempted before
// InitResolver has run. Failing closed turns a wiring mistake into a denied
// request instead of a nil-pointer panic on the request path.
var errResolverNotInitialized = errors.New("acl resolver is not initialized")

// callerGroups returns the groups of the context identity, and fails when the
// resolver they would be resolved against is missing.
func callerGroups(ctx context.Context) ([]string, error) {
	groups, err := identityGroups(ctx)
	if err != nil {
		return nil, err
	}
	if resolver == nil {
		return nil, errResolverNotInitialized
	}
	return groups, nil
}

// Refresher returns the ACL snapshot refresher, exposing reload health for
// readiness checks and allowing callers to force a refresh.
func Refresher() *aclstorev2.Refresher { return refresher }

// identityGroups extracts the group list from the context identity. Every
// identity type (user, service, cluster) resolves through the same groups.
func identityGroups(ctx context.Context) ([]string, error) {
	identity, err := rorcontext.GetIdentityFromRorContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity from context: %w", err)
	}

	return identity.GetGroups()
}

// ClusterSelfAccess is the access a cluster has to its own resources. It used to
// be hardcoded as implicit access; it is now stored as an ACL grant so clusters
// resolve through the same path as every other identity.
func ClusterSelfAccess() []aclmodels.AccessTypeV3 {
	return []aclmodels.AccessTypeV3{
		aclmodels.CapRor.WithVerb(aclmodels.VerbRead),
		aclmodels.CapRor.WithVerb(aclmodels.VerbCreate),
		aclmodels.CapRor.WithVerb(aclmodels.VerbUpdate),
	}
}

// EnsureClusterSelfGrant creates the cluster's self-access ACL entry unless it
// already exists. Without it a cluster identity has no access at all, so this
// must run whenever a cluster uid is established.
func EnsureClusterSelfGrant(ctx context.Context, clusterUID string) error {
	if clusterUID == "" {
		return fmt.Errorf("cluster uid is required to create a self grant")
	}

	group := aclprincipal.Cluster(clusterUID)
	existing, err := Store().GetByGroups(ctx, []string{group})
	if err != nil {
		return fmt.Errorf("could not look up cluster self grant: %w", err)
	}
	for _, entry := range existing {
		if entry.Scope == aclscope.ScopeCluster && entry.Subject == aclscope.Subject(clusterUID) {
			return nil
		}
	}

	_, err = Store().Create(ctx, aclmodels.AclV3ListItem{
		Version: 3,
		Group:   group,
		Scope:   aclscope.ScopeCluster,
		Subject: aclscope.Subject(clusterUID),
		Access:  ClusterSelfAccess(),
		Created: time.Now(),
	})
	if err != nil {
		return fmt.Errorf("could not create cluster self grant: %w", err)
	}
	return nil
}

// resolveClusterSubject converts a human-readable cluster id to its uid for
// cluster-scoped checks, mirroring aclmodels.NewAclV2QueryAccessScopeSubject:
// cluster ACL entries are keyed by uid, but callers usually hold the cluster id
// from the request path. Non-cluster scopes and subjects that are already a uid
// pass through unchanged, and resolution is skipped when no resolver is wired.
func resolveClusterSubject(scope aclscope.Scope, subject aclscope.Subject) aclscope.Subject {
	if scope != aclscope.ScopeCluster {
		return subject
	}
	// A uid subject needs no lookup (and avoids a needless db round-trip).
	if _, err := uuid.Parse(string(subject)); err == nil {
		return subject
	}
	if aclmodels.ClusterIdToUidResolver == nil {
		return subject
	}
	if uid := aclmodels.ClusterIdToUidResolver(string(subject)); uid != "" {
		return aclscope.Subject(uid)
	}
	return subject
}

// resolveClusterFilterSubjects augments an ownerref filter's subjects with the
// resolved uid for any cluster-id subject, so cluster-scoped lookups match the
// uid-keyed store. Original subjects are retained, so non-cluster scopes in the
// same filter are unaffected. It is a no-op when the filter has no subject
// restriction, cannot match cluster scope, or no resolver is wired.
func resolveClusterFilterSubjects(filter acl.OwnerrefFilter) acl.OwnerrefFilter {
	if len(filter.Subjects) == 0 || aclmodels.ClusterIdToUidResolver == nil {
		return filter
	}
	if len(filter.Scopes) > 0 && !slices.Contains(filter.Scopes, aclscope.ScopeCluster) {
		return filter
	}

	subjects := append([]aclscope.Subject(nil), filter.Subjects...)
	for _, s := range filter.Subjects {
		// A uid subject needs no lookup.
		if _, err := uuid.Parse(string(s)); err == nil {
			continue
		}
		uid := aclmodels.ClusterIdToUidResolver(string(s))
		if uid == "" || slices.Contains(subjects, aclscope.Subject(uid)) {
			continue
		}
		subjects = append(subjects, aclscope.Subject(uid))
	}
	filter.Subjects = subjects
	return filter
}

// HasAccess checks if the caller (from context) has the required access type
// for the given scope and subject.
func HasAccess(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject, required aclmodels.AccessTypeV3) (bool, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.HasAccess")
	defer span.End()

	// Cluster ACL entries are keyed by cluster uid; callers commonly pass the
	// cluster id, so resolve it before consulting the store.
	subject = resolveClusterSubject(scope, subject)

	groups, err := callerGroups(ctx)
	if err != nil {
		return false, err
	}

	// Direct grant: exact scope+subject, a global grant, or a scope-level (ror) grant.
	direct, err := resolver.HasAccess(ctx, groups, scope, subject, required)
	if err != nil {
		return false, err
	}
	if direct {
		return true, nil
	}

	// Inherited grant: scope+subject is a descendant (resolved by the scope
	// expander) of an ownerref the caller has direct access to.
	refs, err := resolver.ResolveOwnerrefs(ctx, groups, required, acl.OwnerrefFilter{
		Scopes:   []aclscope.Scope{scope},
		Subjects: []aclscope.Subject{subject},
	})
	if err != nil {
		return false, err
	}
	// nil signals unrestricted access (already covered by the direct check above).
	return refs == nil || len(refs) > 0, nil
}

// resolveAccess returns all access types the caller has for the given scope and subject.
func resolveAccess(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) ([]aclmodels.AccessTypeV3, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResolveAccess")
	defer span.End()

	groups, err := callerGroups(ctx)
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
func ResolveOwnerrefs(ctx context.Context, required aclmodels.AccessTypeV3, filter acl.OwnerrefFilter) (refs []acl.Ownerref, unrestricted bool, err error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResolveOwnerrefs")
	defer span.End()

	groups, err := callerGroups(ctx)
	if err != nil {
		return nil, false, err
	}

	// Cluster ACL entries are keyed by uid; resolve any cluster-id subjects in
	// the filter so cluster-scoped lookups match.
	filter = resolveClusterFilterSubjects(filter)

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
func ResourceOwnerFilter(ctx context.Context, required aclmodels.AccessTypeV3) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResourceOwnerFilter")
	defer span.End()

	groups, err := callerGroups(ctx)
	if err != nil {
		return aclstore.DenyAllFilter, err
	}

	refs, err := resolver.ResolveOwnerrefs(ctx, groups, required, acl.OwnerrefFilter{})
	if err != nil {
		return aclstore.DenyAllFilter, fmt.Errorf("failed to resolve ownerrefs: %w", err)
	}

	return aclstore.OwnerrefsToFilter(refs), nil
}

// clusterUIDDenyAllFilter matches no cluster, mirroring the legacy
// CreateClusterACLFilter no-access shape (an impossible uid).
var clusterUIDDenyAllFilter = bson.M{"$match": bson.M{"uid": bson.M{"$in": bson.A{"Unknown-Unauthorized"}}}}

// ClusterUIDFilter returns a MongoDB aggregation $match stage that scopes a
// query on the clusters collection (keyed by top-level uid) to the clusters the
// caller has the required access for. It replaces the legacy
// GetACL2ByIdentityQuery + CreateClusterACLFilter pair.
//
// Access is resolved via the V3 resolver with scope expansion enabled, so a
// grant on a parent scope (e.g. Project or Workspace) also authorizes the
// clusters beneath it. A ror-level "all clusters" grant, or any global grant,
// yields an unrestricted match. Cluster identities match only their own cluster.
func ClusterUIDFilter(ctx context.Context, required aclmodels.AccessTypeV3) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ClusterUIDFilter")
	defer span.End()

	refs, unrestricted, err := ResolveOwnerrefs(ctx, required, acl.OwnerrefFilter{})
	if err != nil {
		return clusterUIDDenyAllFilter, fmt.Errorf("failed to resolve cluster access: %w", err)
	}
	if unrestricted {
		return bson.M{"$match": bson.M{}}, nil
	}

	uids := make([]string, 0, len(refs))
	for _, ref := range refs {
		// A ror-level scope grant over clusters means "see every cluster".
		if ref.Scope == aclscope.ScopeRor && ref.Subject == aclscope.Subject(aclscope.ScopeCluster) {
			return bson.M{"$match": bson.M{}}, nil
		}
		if ref.Scope == aclscope.ScopeCluster {
			uids = append(uids, string(ref.Subject))
		}
	}

	if len(uids) == 0 {
		return clusterUIDDenyAllFilter, nil
	}

	slices.Sort(uids)
	return bson.M{"$match": bson.M{"uid": bson.M{"$in": uids}}}, nil
}

// ResourceTypeReadFilter returns a MongoDB aggregation pipeline stage that excludes
// protected resource kinds the caller is not authorized to read.
//
// Each protected kind's capability is resolved at its type-level subject
// (scope ror, subject = the kind name). matchesScopeSubject makes that resolution
// honor both a type-level grant ({ror, <Kind>}) and a global grant
// ({ror, globalscope} or subject "all"), so no identity is special-cased.
func ResourceTypeReadFilter(ctx context.Context) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResourceTypeReadFilter")
	defer span.End()

	var excluded []string
	for capability, kinds := range aclstore.ProtectedResourceTypes {
		required := capability.WithVerb(aclmodels.VerbRead)
		for _, kind := range kinds {
			access, err := resolveAccess(ctx, aclscope.ScopeRor, aclscope.Subject(kind))
			if err != nil {
				return bson.M{}, err
			}
			if !slices.Contains(access, required) {
				excluded = append(excluded, kind)
			}
		}
	}

	if len(excluded) == 0 {
		return bson.M{}, nil
	}
	slices.Sort(excluded)
	return bson.M{"$match": bson.M{"typemeta.kind": bson.M{"$nin": excluded}}}, nil
}

// ResourceTypeWriteFilter returns a MongoDB aggregation pipeline stage that excludes
// resource kinds the caller is not authorized to write at the given scope and subject.
//
// Cluster identities get no type restriction (empty filter).
func ResourceTypeWriteFilter(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) (bson.M, error) {
	ctx, span := rortracer.StartSpan(ctx, "aclservice.ResourceTypeWriteFilter")
	defer span.End()

	access, err := resolveAccess(ctx, scope, subject)
	if err != nil {
		return bson.M{}, err
	}

	return aclstore.ResourceTypeWriteFilter(access), nil
}

func GetByFilter(ctx context.Context, filter *apicontracts.Filter) (*apicontracts.PaginatedResult[aclmodels.AclV2ListItem], error) {
	acl, totalCount, err := getByFilter(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("error when getting acl by filter from repo: %v", err)
	}

	paginatedResult := apicontracts.PaginatedResult[aclmodels.AclV2ListItem]{}

	paginatedResult.Data = acl
	paginatedResult.DataCount = int64(len(acl))
	paginatedResult.Offset = int64(filter.Skip)
	paginatedResult.TotalCount = int64(totalCount)

	return &paginatedResult, nil
}

func Create(ctx context.Context, aclModel *aclmodels.AclV2ListItem, identity *identitymodels.Identity) (*aclmodels.AclV2ListItem, error) {
	aclModel.Created = time.Now()
	created, err := Store().Create(ctx, aclmodels.V2ToV3(*aclModel))
	if err != nil {
		return nil, fmt.Errorf("could not create acl: %v", err)
	}
	object := aclmodels.V3ToV2(*created)

	_, err = auditlog.Create(ctx, "ACL created", models.AuditCategoryAcl, models.AuditActionCreate, identity.User, &object, nil)
	if err != nil {
		return nil, fmt.Errorf("could not audit log create action: %v", err)
	}

	return &object, nil
}

func Update(ctx context.Context, aclId string, aclModel *aclmodels.AclV2ListItem, identity *identitymodels.Identity) (*aclmodels.AclV2ListItem, error) {
	updated, previous, err := Store().Update(ctx, aclId, aclmodels.V2ToV3(*aclModel))
	if err != nil {
		return nil, fmt.Errorf("could not update acl: %v", err)
	}
	object := aclmodels.V3ToV2(*updated)

	var oldObject *aclmodels.AclV2ListItem
	if previous != nil {
		o := aclmodels.V3ToV2(*previous)
		oldObject = &o
	}

	_, err = auditlog.Create(ctx, "ACL updated", models.AuditCategoryAcl, models.AuditActionUpdate, identity.User, &object, oldObject)
	if err != nil {
		return nil, fmt.Errorf("could not audit log: %v", err)
	}

	return &object, nil
}

func Delete(ctx context.Context, aclId string, identity *identitymodels.Identity) (bool, *aclmodels.AclV2ListItem, error) {
	if !identity.IsUser() {
		return false, nil, fmt.Errorf("could not delete object, must be delete by a user")
	}

	deleted, err := Store().Delete(ctx, aclId)
	if err != nil {
		return false, nil, fmt.Errorf("could not delete object: %v", err)
	}

	var deletedObject *aclmodels.AclV2ListItem
	if deleted != nil {
		o := aclmodels.V3ToV2(*deleted)
		deletedObject = &o
	}

	_, err = auditlog.Create(ctx, "Acl deleted", models.AuditCategoryAcl, models.AuditActionDelete, identity.User, deletedObject, nil)
	if err != nil {
		return false, nil, fmt.Errorf("could not audit log delete action: %v", err)
	}

	return true, deletedObject, nil
}
