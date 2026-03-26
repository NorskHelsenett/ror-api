// Integration tests for the resourcesv2 database layer.
//
// These tests require a running MongoDB instance. Connection details are
// loaded from the .env file at the repository root (MONGODB_HOST, MONGODB_PORT)
// with docker-compose default credentials. The MONGODB_TEST_URI env var
// overrides the constructed URI if set.
//
// To run:
//
//	docker compose up -d mongodb
//	go test -v -timeout 30s ./internal/apiservices/resourcesv2service/
package resourcesv2service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/rorresourceowner"

	"github.com/joho/godotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// testCredHelper is a no-op credential helper for testing.
type testCredHelper struct{}

func (testCredHelper) GetUsername() string              { return "" }
func (testCredHelper) GetPassword() string              { return "" }
func (testCredHelper) GetCredentials() (string, string) { return "", "" }
func (testCredHelper) CheckAndRenew() bool              { return false }

const testClusterID = "test-cluster-43232"

// testCtx returns a context with a cluster identity matching testClusterID.
// The ACL layer skips MongoDB queries for cluster identities, making it safe
// for use in database-only integration tests.
func testCtx() context.Context {
	identity := identitymodels.Identity{
		Type: identitymodels.IdentityTypeCluster,
		ClusterIdentity: &identitymodels.ServiceIdentity{
			Id: testClusterID,
		},
		ServiceIdentity: &identitymodels.ServiceIdentity{},
	}
	return context.WithValue(context.Background(), identitymodels.ContexIdentity, identity)
}

// newTestRepo connects to a MongoDB instance and returns a ResourceMongoDB
// backed by a unique test database. The database is dropped on test cleanup.
//
// Connection is resolved in order:
//  1. MONGODB_TEST_URI env var (full override)
//  2. Constructed from .env vars (MONGODB_HOST, MONGODB_PORT) + docker-compose
//     default credentials (someone / S3cret!)
func newTestRepo(t *testing.T) *ResourceMongoDB {
	t.Helper()

	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		// Load .env from repo root (3 dirs up from this file's directory)
		_, thisFile, _, _ := runtime.Caller(0)
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
		_ = godotenv.Load(filepath.Join(repoRoot, ".env"))

		host := envOrDefault("MONGODB_HOST", "localhost")
		port := envOrDefault("MONGODB_PORT", "27017")
		user := envOrDefault("MONGO_INITDB_ROOT_USERNAME", "someone")
		pass := envOrDefault("MONGO_INITDB_ROOT_PASSWORD", "S3cret!")

		uri = fmt.Sprintf("mongodb://%s:%s@%s:%s",
			url.PathEscape(user), url.PathEscape(pass), host, port)
	}

	ctx := context.Background()
	opts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(opts)
	if err != nil {
		t.Skipf("MongoDB not available at %s: %v", uri, err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		t.Skipf("MongoDB not reachable at %s: %v", uri, err)
	}

	dbName := fmt.Sprintf("rortest_%s", t.Name())
	db := &mongodb.MongodbCon{
		Client:      client,
		Database:    dbName,
		Credentials: testCredHelper{},
	}

	t.Cleanup(func() {
		_ = client.Database(dbName).Drop(ctx)
		_ = client.Disconnect(ctx)
	})

	return &ResourceMongoDB{db: db}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// --- helpers ---

func makeResource(uid string, kind string, labels map[string]string, annotations map[string]string) *rorresources.Resource {
	r := &rorresources.Resource{}
	r.TypeMeta = metav1.TypeMeta{Kind: kind, APIVersion: "v1"}
	r.Metadata = metav1.ObjectMeta{
		UID:         types.UID(uid),
		Name:        "test-" + uid,
		Namespace:   "default",
		Labels:      labels,
		Annotations: annotations,
	}
	r.RorMeta = rortypes.ResourceRorMeta{
		Version: "v2",
		Hash:    "testhash123",
		Ownerref: rorresourceowner.RorResourceOwnerReference{
			Scope:   aclmodels.Acl2ScopeCluster,
			Subject: aclmodels.Acl2Subject(testClusterID),
		},
	}
	return r
}

func makePodResource(uid string, labels map[string]string, annotations map[string]string, phase string) *rorresources.Resource {
	r := makeResource(uid, "Pod", labels, annotations)
	r.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{
			Phase:   phase,
			Message: "test message",
		},
	}
	return r
}

// --- Set tests ---

func TestSet_CreatesNewResource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-001", map[string]string{"app": "test"}, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-001")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resources, 1)
	assert.Equal(t, "uid-001", result.Resources[0].GetUID())
	assert.Equal(t, "test-uid-001", result.Resources[0].GetName())
}

func TestSet_WithDottedLabels(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	labels := map[string]string{
		"app":                         "metallb",
		"app.kubernetes.io/component": "controller",
		"app.kubernetes.io/instance":  "metallb-system",
		"app.kubernetes.io/name":      "metallb",
	}

	resource := makePodResource("uid-dotted", labels, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err, "Set should handle dotted label keys")

	query := rorresources.NewResourceQuery().WithUID("uid-dotted")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "metallb", result.Resources[0].Metadata.Labels["app"])
	assert.Equal(t, "controller", result.Resources[0].Metadata.Labels["app.kubernetes.io/component"])
}

func TestSet_WithDottedAnnotations(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	annotations := map[string]string{
		"argocd.argoproj.io/tracking-id":                   "nhn-tooling:argoproj.io/Application:argocd/ror-agent",
		"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"v1"}`,
	}

	resource := makePodResource("uid-annot", nil, annotations, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err, "Set should handle dotted annotation keys")

	query := rorresources.NewResourceQuery().WithUID("uid-annot")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "nhn-tooling:argoproj.io/Application:argocd/ror-agent",
		result.Resources[0].Metadata.Annotations["argocd.argoproj.io/tracking-id"])
}

func TestSet_WithNilAnnotationsAndLabels(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-nil", nil, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	// A second Set should also succeed (the original bug: "Cannot create field in element {annotations: null}")
	resource.PodResource.Status.Phase = "Succeeded"
	err = repo.Set(ctx, resource)
	require.NoError(t, err, "Set should handle nil annotations on existing document")
}

func TestSet_ReplacesFully(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-replace", map[string]string{"app": "v1"}, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	resource.Metadata.Labels["app"] = "v2"
	resource.PodResource.Status.Phase = "Succeeded"
	err = repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-replace")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "v2", result.Resources[0].Metadata.Labels["app"])
}

// --- Patch tests ---

func TestPatch_UpdatesSingleField(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-patch-1", map[string]string{"app": "test"}, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Succeeded"},
	}
	err = repo.Patch(ctx, "uid-patch-1", partial)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-patch-1")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "test", result.Resources[0].Metadata.Labels["app"], "labels should be preserved after patch")
	assert.Equal(t, "test-uid-patch-1", result.Resources[0].GetName(), "name should be preserved after patch")
}

func TestPatch_PreservesDottedLabels(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	labels := map[string]string{
		"app":                         "metallb",
		"app.kubernetes.io/component": "controller",
	}

	resource := makePodResource("uid-patch-dotted", labels, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Succeeded"},
	}
	err = repo.Patch(ctx, "uid-patch-dotted", partial)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-patch-dotted")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "metallb", result.Resources[0].Metadata.Labels["app"])
	assert.Equal(t, "controller", result.Resources[0].Metadata.Labels["app.kubernetes.io/component"],
		"dotted label keys should survive patch")
}

func TestPatch_NonExistentResource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Failed"},
	}

	err := repo.Patch(ctx, "uid-nonexistent", partial)
	require.Error(t, err, "patching a non-existent resource should return an error")
	assert.Contains(t, err.Error(), "not found")
}

func TestPatch_WithDottedAnnotations(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	annotations := map[string]string{
		"argocd.argoproj.io/tracking-id": "original",
	}
	resource := makePodResource("uid-patch-annot", nil, annotations, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	partial := &rorresources.Resource{}
	partial.Metadata = metav1.ObjectMeta{
		Annotations: map[string]string{
			"argocd.argoproj.io/tracking-id": "updated",
		},
	}
	err = repo.Patch(ctx, "uid-patch-annot", partial)
	require.NoError(t, err, "patch should handle dotted annotation keys")

	query := rorresources.NewResourceQuery().WithUID("uid-patch-annot")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "updated",
		result.Resources[0].Metadata.Annotations["argocd.argoproj.io/tracking-id"])
}

// --- Del tests ---

func TestDel_RemovesResource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-del-1", nil, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	err = repo.Del(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-del-1")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	assert.Nil(t, result, "resource should be gone after delete")
}

func TestDel_NonExistentSucceeds(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makeResource("uid-del-missing", "Pod", nil, nil)
	err := repo.Del(ctx, resource)
	require.NoError(t, err, "deleting a non-existent resource should not error")
}

// --- Round trip test ---

func TestRoundTrip_SetGetPatchGetDel(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	labels := map[string]string{
		"app":                          "myapp",
		"app.kubernetes.io/component":  "frontend",
		"app.kubernetes.io/managed-by": "argocd",
	}
	annotations := map[string]string{
		"argocd.argoproj.io/tracking-id":                   "myapp:apps/v1:Deployment:default/myapp",
		"kubectl.kubernetes.io/last-applied-configuration": `{"spec":{}}`,
	}

	// Step 1: Set
	resource := makePodResource("uid-roundtrip", labels, annotations, "Pending")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	// Step 2: Get and verify
	query := rorresources.NewResourceQuery().WithUID("uid-roundtrip")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Resources, 1)
	got := result.Resources[0]
	assert.Equal(t, "Pending", got.PodResource.Status.Phase)
	assert.Equal(t, "frontend", got.Metadata.Labels["app.kubernetes.io/component"])
	assert.Equal(t, "myapp:apps/v1:Deployment:default/myapp",
		got.Metadata.Annotations["argocd.argoproj.io/tracking-id"])

	// Step 3: Patch just the status
	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{
			Phase:   "Running",
			Message: "started",
		},
	}
	err = repo.Patch(ctx, "uid-roundtrip", partial)
	require.NoError(t, err)

	// Step 4: Get and verify patch preserved labels/annotations
	result, err = repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	got = result.Resources[0]
	assert.Equal(t, "Running", got.PodResource.Status.Phase, "phase should be updated")
	assert.Equal(t, "frontend", got.Metadata.Labels["app.kubernetes.io/component"],
		"dotted labels should survive patch")
	assert.Equal(t, "myapp:apps/v1:Deployment:default/myapp",
		got.Metadata.Annotations["argocd.argoproj.io/tracking-id"],
		"dotted annotations should survive patch")
	assert.Equal(t, "test-uid-roundtrip", got.GetName(), "name should survive patch")

	// Step 5: Delete
	err = repo.Del(ctx, resource)
	require.NoError(t, err)

	result, err = repo.Get(ctx, query)
	require.NoError(t, err)
	assert.Nil(t, result, "resource should be deleted")
}
