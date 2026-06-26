// Integration tests for the MongoDB-backed scope expander
// (aclstore.MongoScopeExpander). They verify the ownerref-chain traversal
// against a real MongoDB instance, focusing on the invariant that drives the
// implementation: the expander must only ever return "owner" (parent)
// resources and must never traverse or return leaf resources (e.g. the
// in-cluster resources a KubernetesCluster owns). Pruning leaves at traversal
// time is what keeps the $graphLookup result within MongoDB's memory limit.
//
// These tests require a running MongoDB instance. Connection details are
// loaded from the .env file at the repository root (MONGODB_HOST, MONGODB_PORT)
// with docker-compose default credentials. The MONGODB_TEST_URI env var
// overrides the constructed URI if set. Tests are skipped when no MongoDB is
// reachable.
//
// To run:
//
//	docker compose up -d mongodb
//	go test -v -timeout 60s ./internal/acl/aclservice/v2/
package aclservice

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/acl/aclstore"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// resourcesV2Collection mirrors the unexported collection name used by the
// expander (aclstore.resourceV2Collection).
const resourcesV2Collection = "resourcesv2"

// newExpanderTestDB connects to a MongoDB instance and returns a database
// backed scope expander plus the underlying database for seeding. The database
// is unique per test and dropped on cleanup.
//
// Connection is resolved in order:
//  1. MONGODB_TEST_URI env var (full override)
//  2. Constructed from .env vars (MONGODB_HOST, MONGODB_PORT) + docker-compose
//     default credentials (someone / S3cret!)
func newExpanderTestDB(t *testing.T) (*mongo.Database, *aclstore.MongoScopeExpander) {
	t.Helper()

	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		// Load .env from repo root (4 dirs up from this file's directory).
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
		_ = godotenv.Load(filepath.Join(repoRoot, ".env"))

		host := expanderEnvOrDefault("MONGODB_HOST", "localhost")
		port := expanderEnvOrDefault("MONGODB_PORT", "27017")
		user := expanderEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "someone")
		pass := expanderEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "S3cret!")

		uri = fmt.Sprintf("mongodb://%s:%s@%s:%s",
			url.PathEscape(user), url.PathEscape(pass), host, port)
	}

	ctx := context.Background()
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Skipf("MongoDB not available at %s: %v", uri, err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		t.Skipf("MongoDB not reachable at %s: %v", uri, err)
	}

	db := client.Database(expanderTestDBName(t))

	t.Cleanup(func() {
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	return db, aclstore.NewMongoScopeExpander(func() *mongo.Database { return db })
}

func expanderEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// expanderTestDBName derives a short, valid MongoDB database name from the test
// name (which can otherwise be too long / contain invalid characters).
func expanderTestDBName(t *testing.T) string {
	name := strings.NewReplacer(
		"TestMongoScopeExpander_", "",
		"/", "_",
		" ", "_",
	).Replace(t.Name())
	if len(name) > 48 {
		name = name[:48]
	}
	return "rortest_exp_" + name
}

// seedResource inserts a minimal resourcesv2 document carrying only the fields
// the expander reads: the resource's own uid + kind, and the ownerref it points
// at. The ownership tree is derived entirely from these.
func seedResource(t *testing.T, db *mongo.Database, uid, kind string, ownerScope aclscope.Scope, ownerSubject string) {
	t.Helper()
	_, err := db.Collection(resourcesV2Collection).InsertOne(context.Background(), bson.D{
		{Key: "uid", Value: uid},
		{Key: "typemeta", Value: bson.D{{Key: "kind", Value: kind}}},
		{Key: "rormeta", Value: bson.D{{Key: "ownerref", Value: bson.D{
			{Key: "scope", Value: string(ownerScope)},
			{Key: "subject", Value: ownerSubject},
		}}}},
	})
	require.NoError(t, err)
}

// seedLeaves inserts n leaf resources of the given kind, all owned by owner.
func seedLeaves(t *testing.T, db *mongo.Database, kind string, ownerSubject string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		seedResource(t, db, fmt.Sprintf("%s-%s-%d", strings.ToLower(kind), ownerSubject, i), kind, aclscope.ScopeCluster, ownerSubject)
	}
}

func clusterRef(uid string) acl.Ownerref {
	return acl.Ownerref{Scope: aclscope.ScopeCluster, Subject: aclscope.Subject(uid)}
}

func projectRef(uid string) acl.Ownerref {
	return acl.Ownerref{Scope: aclscope.ScopeProject, Subject: aclscope.Subject(uid)}
}

// --- ExpandScope ---

// A cluster's leaf children (Pods etc.) must never appear in the expansion: a
// scope grant on a project resolves to the clusters it owns, not to the tens of
// thousands of in-cluster resources beneath them.
func TestMongoScopeExpander_ExpandScope_ExcludesLeafChildren(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "proj-1", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedLeaves(t, db, "Pod", "cluster-1", 50)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeProject, "proj-1")
	require.NoError(t, err)

	assert.Equal(t, []acl.Ownerref{clusterRef("cluster-1")}, refs,
		"project expansion should yield its cluster only, never the cluster's leaf children")
}

// Owner descendants must be collected across every level of the hierarchy
// (datacenter -> project -> cluster) while leaves stay excluded.
func TestMongoScopeExpander_ExpandScope_MultiLevelHierarchy(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "dc-1", "Datacenter", aclscope.ScopeRor, "ror")
	seedResource(t, db, "proj-1", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "proj-2", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedResource(t, db, "cluster-2", "KubernetesCluster", aclscope.ScopeProject, "proj-2")
	seedLeaves(t, db, "Pod", "cluster-1", 20)
	seedLeaves(t, db, "ConfigMap", "cluster-2", 20)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeDatacenter, "dc-1")
	require.NoError(t, err)

	assert.ElementsMatch(t, []acl.Ownerref{
		projectRef("proj-1"),
		projectRef("proj-2"),
		clusterRef("cluster-1"),
		clusterRef("cluster-2"),
	}, refs, "datacenter expansion should yield all descendant projects and clusters, but no leaves")
}

// A (non self-owned) cluster that owns only leaf resources expands to nothing:
// it is the bottom owner in the chain and there is no owned scope below it.
func TestMongoScopeExpander_ExpandScope_ClusterWithOnlyLeaves_ReturnsNil(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "proj-1", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedLeaves(t, db, "Pod", "cluster-1", 100)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeCluster, "cluster-1")
	require.NoError(t, err)
	assert.Nil(t, refs, "a cluster owning only leaves must expand to nil, not to its leaf children")
}

// Production clusters are self-owned roots (ownerref.subject == own uid). The
// self-loop means the cluster appears as its own descendant, but its leaves are
// still pruned — so the result is exactly the cluster itself.
func TestMongoScopeExpander_ExpandScope_SelfOwnedRoot_ReturnsSelfWithoutLeaves(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "cluster-self", "KubernetesCluster", aclscope.ScopeCluster, "cluster-self")
	seedLeaves(t, db, "Pod", "cluster-self", 100)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeCluster, "cluster-self")
	require.NoError(t, err)
	assert.Equal(t, []acl.Ownerref{clusterRef("cluster-self")}, refs,
		"self-owned cluster expands to itself only; its leaf children must be pruned")
}

// A leaf resource owns nothing, so seeding it as a scope must expand to nil.
func TestMongoScopeExpander_ExpandScope_LeafSeed_ReturnsNil(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedResource(t, db, "pod-1", "Pod", aclscope.ScopeCluster, "cluster-1")

	refs, err := expander.ExpandScope(context.Background(), aclscope.Scope("Pod"), "pod-1")
	require.NoError(t, err)
	assert.Nil(t, refs, "a leaf resource owns nothing and must expand to nil")
}

// Edge case of the relational "parent" definition: an owner resource that owns
// nothing (e.g. a freshly registered cluster whose resources have not synced
// yet) is indistinguishable from a leaf — nobody references it as an
// ownerref.subject — and is therefore NOT returned by its parent's expansion.
// This documents the intentional behavior so it is not mistaken for a bug.
func TestMongoScopeExpander_ExpandScope_ChildlessOwnerResource_IsExcluded(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "proj-1", "Project", aclscope.ScopeDatacenter, "dc-1")
	// cluster-empty is owned by proj-1 but owns nothing itself.
	seedResource(t, db, "cluster-empty", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	// cluster-full is owned by proj-1 and owns leaves, so it is a real owner.
	seedResource(t, db, "cluster-full", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedLeaves(t, db, "Pod", "cluster-full", 5)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeProject, "proj-1")
	require.NoError(t, err)
	assert.Equal(t, []acl.Ownerref{clusterRef("cluster-full")}, refs,
		"a childless owner resource is treated as a leaf and excluded; only resources that actually own something are returned")
}

// A subject with no resources at all must expand to nil without error.
func TestMongoScopeExpander_ExpandScope_UnknownSubject_ReturnsNil(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedLeaves(t, db, "Pod", "cluster-1", 5)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeProject, "does-not-exist")
	require.NoError(t, err)
	assert.Nil(t, refs)
}

// With no resources at all the pipeline must still succeed and return nil.
func TestMongoScopeExpander_ExpandScope_EmptyCollection_ReturnsNil(t *testing.T) {
	_, expander := newExpanderTestDB(t)

	refs, err := expander.ExpandScope(context.Background(), aclscope.ScopeProject, "proj-1")
	require.NoError(t, err)
	assert.Nil(t, refs)
}

// --- ExpandScopes (batched) ---

// Each seed in a batch must get its own correct, independent owner descendants,
// including seeds that resolve to nothing.
func TestMongoScopeExpander_ExpandScopes_BatchPerSeed(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	// Two independent subtrees plus an empty seed.
	seedResource(t, db, "proj-1", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedLeaves(t, db, "Pod", "cluster-1", 10)

	seedResource(t, db, "proj-2", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "cluster-2a", "KubernetesCluster", aclscope.ScopeProject, "proj-2")
	seedResource(t, db, "cluster-2b", "KubernetesCluster", aclscope.ScopeProject, "proj-2")
	seedLeaves(t, db, "Pod", "cluster-2a", 10)
	seedLeaves(t, db, "Pod", "cluster-2b", 10)

	seed1 := projectRef("proj-1")
	seed2 := projectRef("proj-2")
	seedEmpty := projectRef("proj-empty")

	got, err := expander.ExpandScopes(context.Background(), []acl.Ownerref{seed1, seed2, seedEmpty})
	require.NoError(t, err)

	require.Contains(t, got, seed1)
	require.Contains(t, got, seed2)
	require.Contains(t, got, seedEmpty)

	assert.ElementsMatch(t, []acl.Ownerref{clusterRef("cluster-1")}, got[seed1])
	assert.ElementsMatch(t, []acl.Ownerref{clusterRef("cluster-2a"), clusterRef("cluster-2b")}, got[seed2])
	assert.Empty(t, got[seedEmpty], "seed with no descendants must be present with no owners")
}

// An empty seed slice short-circuits to an empty map without touching MongoDB.
func TestMongoScopeExpander_ExpandScopes_EmptySeeds_ReturnsEmptyMap(t *testing.T) {
	_, expander := newExpanderTestDB(t)

	got, err := expander.ExpandScopes(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

// Duplicate seeds (same subject) must collapse to a single key.
func TestMongoScopeExpander_ExpandScopes_DeduplicatesSeeds(t *testing.T) {
	db, expander := newExpanderTestDB(t)

	seedResource(t, db, "proj-1", "Project", aclscope.ScopeDatacenter, "dc-1")
	seedResource(t, db, "cluster-1", "KubernetesCluster", aclscope.ScopeProject, "proj-1")
	seedLeaves(t, db, "Pod", "cluster-1", 5)

	seed := projectRef("proj-1")
	got, err := expander.ExpandScopes(context.Background(), []acl.Ownerref{seed, seed})
	require.NoError(t, err)

	require.Len(t, got, 1)
	assert.ElementsMatch(t, []acl.Ownerref{clusterRef("cluster-1")}, got[seed])
}
