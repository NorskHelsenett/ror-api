// Integration tests for the MongoDB-backed ACL store (MongoStore). They verify
// the full read/write surface against a real MongoDB instance, including that
// legacy V2 documents are transparently converted to canonical V3 on read.
//
// These tests require a running MongoDB instance. Connection details are loaded
// from the .env file at the repository root (MONGODB_HOST, MONGODB_PORT) with
// docker-compose default credentials. The MONGODB_TEST_URI env var overrides
// the constructed URI if set. Tests are skipped when no MongoDB is reachable.
//
// To run:
//
//	docker compose up -d mongodb
//	go test -v -timeout 60s ./internal/acl/aclstore/
package aclstore

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

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// newStoreTestDB connects to a MongoDB instance and returns a MongoStore plus
// the underlying database for seeding. The database is unique per test and
// dropped on cleanup.
//
// Connection is resolved in order:
//  1. MONGODB_TEST_URI env var (full override)
//  2. Constructed from .env vars (MONGODB_HOST, MONGODB_PORT) + docker-compose
//     default credentials (someone / S3cret!)
func newStoreTestDB(t *testing.T) (*mongo.Database, *MongoStore) {
	t.Helper()

	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		// Load .env from repo root (3 dirs up from this file's directory).
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
		_ = godotenv.Load(filepath.Join(repoRoot, ".env"))

		host := storeEnvOrDefault("MONGODB_HOST", "localhost")
		port := storeEnvOrDefault("MONGODB_PORT", "27017")
		user := storeEnvOrDefault("MONGO_INITDB_ROOT_USERNAME", "someone")
		pass := storeEnvOrDefault("MONGO_INITDB_ROOT_PASSWORD", "S3cret!")

		uri = fmt.Sprintf("mongodb://%s:%s@%s:%s",
			url.PathEscape(user), url.PathEscape(pass), host, port)
	}

	ctx := context.Background()
	client, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetBSONOptions(&options.BSONOptions{ObjectIDAsHexString: true}))
	if err != nil {
		t.Skipf("MongoDB not available at %s: %v", uri, err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		t.Skipf("MongoDB not reachable at %s: %v", uri, err)
	}

	db := client.Database(storeTestDBName(t))

	t.Cleanup(func() {
		_ = db.Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	return db, NewMongoStore(func() *mongo.Database { return db })
}

func storeEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// storeTestDBName derives a short, valid MongoDB database name from the test name.
func storeTestDBName(t *testing.T) string {
	name := strings.NewReplacer(
		"TestMongoStore_", "",
		"/", "_",
		" ", "_",
	).Replace(t.Name())
	if len(name) > 48 {
		name = name[:48]
	}
	return "rortest_store_" + name
}

// v3Entry builds a minimal valid V3 ACL entry for seeding via the store.
func v3Entry(group string, scope aclscope.Scope, subject aclscope.Subject, access ...aclmodels.AccessTypeV3) aclmodels.AclV3ListItem {
	return aclmodels.AclV3ListItem{
		Group:   group,
		Scope:   scope,
		Subject: subject,
		Access:  access,
	}
}

// seedV2 inserts a legacy V2 document directly, bypassing the store, to exercise
// the V2 to V3 read/write conversion. It returns the inserted document's hex id.
func seedV2(t *testing.T, db *mongo.Database, group string, scope aclscope.Scope, subject aclscope.Subject) string {
	t.Helper()
	v2 := aclmodels.AclV2ListItem{
		Version: 2,
		Group:   group,
		Scope:   scope,
		Subject: subject,
		Access:  aclmodels.AclV2ListItemAccess{Read: true},
		Created: time.Now(),
	}
	res, err := db.Collection(aclCollectionName).InsertOne(context.Background(), v2)
	require.NoError(t, err)
	oid, ok := res.InsertedID.(bson.ObjectID)
	require.True(t, ok)
	return oid.Hex()
}

// storedVersion reads the raw "version" field of a document directly from mongo,
// bypassing the store's read conversion, to assert what is actually persisted.
func storedVersion(t *testing.T, db *mongo.Database, hexID string) int {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(hexID)
	require.NoError(t, err)
	var doc struct {
		Version int `bson:"version"`
	}
	err = db.Collection(aclCollectionName).FindOne(context.Background(), bson.M{"_id": oid}).Decode(&doc)
	require.NoError(t, err)
	return doc.Version
}

// --- Create / GetById ---

func TestMongoStore_CreateAndGetById(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	created, err := store.Create(ctx, v3Entry("dev-team", aclscope.ScopeCluster, "cluster-1", "ror:read"))
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.NotEmpty(t, created.Id, "store must assign an id")
	assert.Equal(t, 3, created.Version)
	assert.False(t, created.Created.IsZero(), "Created should be set")

	got, err := store.GetById(ctx, created.Id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, created.Id, got.Id)
	assert.Equal(t, "dev-team", got.Group)
	assert.Equal(t, aclscope.ScopeCluster, got.Scope)
	assert.Equal(t, aclscope.Subject("cluster-1"), got.Subject)
	assert.Equal(t, []aclmodels.AccessTypeV3{"ror:read"}, got.Access)
}

func TestMongoStore_GetById_NotFound(t *testing.T) {
	_, store := newStoreTestDB(t)

	// Valid ObjectID hex that does not exist.
	got, err := store.GetById(context.Background(), "0123456789abcdef01234567")
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestMongoStore_GetById_InvalidId(t *testing.T) {
	_, store := newStoreTestDB(t)

	_, err := store.GetById(context.Background(), "not-an-object-id")
	assert.Error(t, err)
}

// --- GetByGroups ---

// countGroup counts entries in a flat list belonging to the given group.
func countGroup(l aclmodels.AclV3List, group string) int {
	n := 0
	for _, e := range l {
		if e.Group == group {
			n++
		}
	}
	return n
}

func TestMongoStore_GetByGroups(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	_, err := store.Create(ctx, v3Entry("team-a", aclscope.ScopeCluster, "c1", "ror:read"))
	require.NoError(t, err)
	_, err = store.Create(ctx, v3Entry("team-a", aclscope.ScopeCluster, "c2", "ror:read"))
	require.NoError(t, err)
	_, err = store.Create(ctx, v3Entry("team-b", aclscope.ScopeProject, "p1", "ror:read"))
	require.NoError(t, err)

	byA, err := store.GetByGroups(ctx, []string{"team-a"})
	require.NoError(t, err)
	assert.Len(t, byA, 2)
	assert.Equal(t, 0, countGroup(byA, "team-b"), "unqueried group must be absent")

	both, err := store.GetByGroups(ctx, []string{"team-a", "team-b"})
	require.NoError(t, err)
	assert.Equal(t, 2, countGroup(both, "team-a"))
	assert.Equal(t, 1, countGroup(both, "team-b"))
}

// --- GetByScopeSubject ---

func TestMongoStore_GetByScopeSubject(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	_, err := store.Create(ctx, v3Entry("team-a", aclscope.ScopeCluster, "c1", "ror:read"))
	require.NoError(t, err)
	_, err = store.Create(ctx, v3Entry("team-b", aclscope.ScopeCluster, "c1", "ror:write"))
	require.NoError(t, err)
	_, err = store.Create(ctx, v3Entry("team-a", aclscope.ScopeCluster, "c2", "ror:read"))
	require.NoError(t, err)

	got, err := store.GetByScopeSubject(ctx, aclscope.ScopeCluster, "c1")
	require.NoError(t, err)
	assert.Len(t, got, 2, "both entries for cluster c1 regardless of group")

	none, err := store.GetByScopeSubject(ctx, aclscope.ScopeCluster, "does-not-exist")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// --- GetAll (incl. V2 conversion) ---

func TestMongoStore_GetAll_IncludesConvertedV2(t *testing.T) {
	db, store := newStoreTestDB(t)
	ctx := context.Background()

	_, err := store.Create(ctx, v3Entry("team-a", aclscope.ScopeCluster, "c1", "ror:read", "kubernetes:logon"))
	require.NoError(t, err)
	seedV2(t, db, "legacy-team", aclscope.ScopeCluster, "c-legacy")

	all, err := store.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)

	byGroup := map[string]aclmodels.AclV3ListItem{}
	for _, e := range all {
		byGroup[e.Group] = e
	}

	legacy, ok := byGroup["legacy-team"]
	require.True(t, ok, "legacy V2 entry must be returned")
	assert.Equal(t, 3, legacy.Version, "V2 entry must be converted to V3")
	assert.Equal(t, []aclmodels.AccessTypeV3{"ror:read"}, legacy.Access,
		"V2 Access.Read must map to ror:read")
}

// --- Update ---

func TestMongoStore_Update(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	created, err := store.Create(ctx, v3Entry("dev-team", aclscope.ScopeCluster, "c1", "ror:read"))
	require.NoError(t, err)

	updatedItem := v3Entry("dev-team", aclscope.ScopeCluster, "c1", "ror:read", "ror:write")
	updated, previous, err := store.Update(ctx, created.Id, updatedItem)
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, previous)

	assert.Equal(t, created.Id, updated.Id, "id must be stable across update")
	assert.ElementsMatch(t, []aclmodels.AccessTypeV3{"ror:read", "ror:write"}, updated.Access)
	assert.Equal(t, []aclmodels.AccessTypeV3{"ror:read"}, previous.Access, "previous must reflect pre-update state")
	assert.Equal(t, created.Created, updated.Created, "Created must be preserved")

	// Confirm persisted.
	got, err := store.GetById(ctx, created.Id)
	require.NoError(t, err)
	assert.ElementsMatch(t, []aclmodels.AccessTypeV3{"ror:read", "ror:write"}, got.Access)
}

func TestMongoStore_Update_NotFound(t *testing.T) {
	_, store := newStoreTestDB(t)

	_, _, err := store.Update(context.Background(), "0123456789abcdef01234567",
		v3Entry("x", aclscope.ScopeCluster, "c1", "ror:read"))
	assert.Error(t, err)
}

// --- Delete ---

func TestMongoStore_Delete(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	created, err := store.Create(ctx, v3Entry("dev-team", aclscope.ScopeCluster, "c1", "ror:read"))
	require.NoError(t, err)

	deleted, err := store.Delete(ctx, created.Id)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, created.Id, deleted.Id)

	got, err := store.GetById(ctx, created.Id)
	require.NoError(t, err)
	assert.Nil(t, got, "entry must be gone after delete")
}

func TestMongoStore_Delete_NotFound(t *testing.T) {
	_, store := newStoreTestDB(t)

	_, err := store.Delete(context.Background(), "0123456789abcdef01234567")
	assert.Error(t, err)
}

// --- Canonical write model ---

// Create must ignore a caller-supplied id and version: it always assigns a fresh
// id and persists as V3.
func TestMongoStore_Create_EnforcesCanonical(t *testing.T) {
	db, store := newStoreTestDB(t)
	ctx := context.Background()

	item := v3Entry("dev-team", aclscope.ScopeCluster, "c1", "ror:read")
	item.Id = "deadbeefdeadbeefdeadbeef" // should be ignored
	item.Version = 2                     // should be forced to 3

	created, err := store.Create(ctx, item)
	require.NoError(t, err)
	assert.NotEqual(t, "deadbeefdeadbeefdeadbeef", created.Id, "store must assign a fresh id")
	assert.Equal(t, 3, created.Version)
	assert.Equal(t, 3, storedVersion(t, db, created.Id), "document must be persisted as V3")
}

// Update must not let the caller override the immutable Created / IssuedBy fields.
func TestMongoStore_Update_PreservesImmutableFields(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	seed := v3Entry("dev-team", aclscope.ScopeCluster, "c1", "ror:read")
	seed.IssuedBy = "original@example.com"
	created, err := store.Create(ctx, seed)
	require.NoError(t, err)

	change := v3Entry("dev-team", aclscope.ScopeCluster, "c1", "ror:read", "ror:write")
	change.Created = time.Now().Add(48 * time.Hour) // should be ignored
	change.IssuedBy = "attacker@example.com"        // should be ignored

	updated, _, err := store.Update(ctx, created.Id, change)
	require.NoError(t, err)
	assert.Equal(t, created.Created, updated.Created, "Created must be preserved from the original")
	assert.Equal(t, "original@example.com", updated.IssuedBy, "IssuedBy must be preserved from the original")
}

// --- Membership change (drives cache invalidation) ---

// Moving an entry to a different group must be reflected in GetByGroups, and the
// previous item returned by Update must still carry the old group so callers can
// invalidate both.
func TestMongoStore_Update_ChangesGroup(t *testing.T) {
	_, store := newStoreTestDB(t)
	ctx := context.Background()

	created, err := store.Create(ctx, v3Entry("old-group", aclscope.ScopeCluster, "c1", "ror:read"))
	require.NoError(t, err)

	updated, previous, err := store.Update(ctx, created.Id,
		v3Entry("new-group", aclscope.ScopeCluster, "c1", "ror:read"))
	require.NoError(t, err)
	assert.Equal(t, "old-group", previous.Group, "previous must retain the old group")
	assert.Equal(t, "new-group", updated.Group)

	byOld, err := store.GetByGroups(ctx, []string{"old-group"})
	require.NoError(t, err)
	assert.Empty(t, byOld, "entry must no longer appear under the old group")

	byNew, err := store.GetByGroups(ctx, []string{"new-group"})
	require.NoError(t, err)
	assert.Len(t, byNew, 1)
}

// --- Legacy V2 read/write paths ---

func TestMongoStore_GetById_ConvertsV2(t *testing.T) {
	db, store := newStoreTestDB(t)

	id := seedV2(t, db, "legacy-team", aclscope.ScopeCluster, "c-legacy")

	got, err := store.GetById(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, 3, got.Version, "V2 doc must be returned as V3")
	assert.Equal(t, []aclmodels.AccessTypeV3{"ror:read"}, got.Access)
}

// Updating a legacy V2 document migrates it to V3 in place.
func TestMongoStore_Update_MigratesV2(t *testing.T) {
	db, store := newStoreTestDB(t)
	ctx := context.Background()

	id := seedV2(t, db, "legacy-team", aclscope.ScopeCluster, "c-legacy")
	require.Equal(t, 2, storedVersion(t, db, id), "precondition: doc starts as V2")

	updated, previous, err := store.Update(ctx, id,
		v3Entry("legacy-team", aclscope.ScopeCluster, "c-legacy", "ror:read", "ror:write"))
	require.NoError(t, err)
	assert.Equal(t, []aclmodels.AccessTypeV3{"ror:read"}, previous.Access, "previous reflects converted V2 state")
	assert.ElementsMatch(t, []aclmodels.AccessTypeV3{"ror:read", "ror:write"}, updated.Access)
	assert.Equal(t, 3, storedVersion(t, db, id), "doc must be migrated to V3 in place")
}

func TestMongoStore_Delete_ConvertsV2(t *testing.T) {
	db, store := newStoreTestDB(t)

	id := seedV2(t, db, "legacy-team", aclscope.ScopeCluster, "c-legacy")

	deleted, err := store.Delete(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, deleted)
	assert.Equal(t, 3, deleted.Version, "deleted item must be returned as V3")
	assert.Equal(t, []aclmodels.AccessTypeV3{"ror:read"}, deleted.Access)
}

// --- Empty collection ---

func TestMongoStore_GetAll_Empty(t *testing.T) {
	_, store := newStoreTestDB(t)

	all, err := store.GetAll(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all)
}
