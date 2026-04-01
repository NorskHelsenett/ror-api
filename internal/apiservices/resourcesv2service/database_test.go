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
	"go.mongodb.org/mongo-driver/v2/bson"
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

// --- Set edge cases ---

func TestSet_Idempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-idem", map[string]string{"app": "test"}, nil, "Running")

	// Set the same resource 3 times — should always succeed with the same data
	for i := 0; i < 3; i++ {
		err := repo.Set(ctx, resource)
		require.NoError(t, err, "Set should be idempotent (attempt %d)", i+1)
	}

	query := rorresources.NewResourceQuery().WithUID("uid-idem")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resources, 1)
}

func TestSet_RemovesLabelsOnReplace(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	// First Set with labels
	resource := makePodResource("uid-strip-labels", map[string]string{"app": "v1", "env": "prod"}, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	// Second Set without labels — ReplaceOne should remove them
	resource.Metadata.Labels = nil
	err = repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-strip-labels")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Resources[0].Metadata.Labels, "labels should be nil after replace with no labels")
}

func TestSet_DeploymentResource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makeResource("uid-deploy", "Deployment", map[string]string{"app": "nginx"}, nil)
	resource.TypeMeta.APIVersion = "apps/v1"
	resource.DeploymentResource = &rortypes.ResourceDeployment{
		Status: rortypes.ResourceDeploymentStatus{
			Replicas:          3,
			AvailableReplicas: 3,
			ReadyReplicas:     3,
			UpdatedReplicas:   3,
		},
	}

	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-deploy")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Deployment", result.Resources[0].Kind)
	assert.Equal(t, 3, result.Resources[0].DeploymentResource.Status.Replicas)
}

func TestSet_WithEmptyLabelsMap(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	// Empty map (not nil) — should behave differently from nil
	resource := makePodResource("uid-empty-labels", map[string]string{}, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	// Set again — should not fail
	resource.PodResource.Status.Phase = "Succeeded"
	err = repo.Set(ctx, resource)
	require.NoError(t, err, "Set should handle empty (non-nil) labels map")
}

func TestSet_WithBothDottedLabelsAndAnnotations(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	labels := map[string]string{
		"app":                         "myapp",
		"app.kubernetes.io/name":      "myapp",
		"app.kubernetes.io/component": "backend",
	}
	annotations := map[string]string{
		"argocd.argoproj.io/tracking-id":                   "tracking-id",
		"kubectl.kubernetes.io/last-applied-configuration": `{"spec":{}}`,
		"cert-manager.io/cluster-issuer":                   "letsencrypt",
	}

	resource := makePodResource("uid-both-dotted", labels, annotations, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-both-dotted")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	got := result.Resources[0]
	assert.Equal(t, "backend", got.Metadata.Labels["app.kubernetes.io/component"])
	assert.Equal(t, "tracking-id", got.Metadata.Annotations["argocd.argoproj.io/tracking-id"])
	assert.Equal(t, "letsencrypt", got.Metadata.Annotations["cert-manager.io/cluster-issuer"])
}

func TestSet_LargeLabelsCount(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	labels := make(map[string]string, 50)
	for i := 0; i < 50; i++ {
		labels[fmt.Sprintf("label-%d", i)] = fmt.Sprintf("value-%d", i)
	}

	resource := makePodResource("uid-many-labels", labels, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-many-labels")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resources[0].Metadata.Labels, 50)
	assert.Equal(t, "value-0", result.Resources[0].Metadata.Labels["label-0"])
	assert.Equal(t, "value-49", result.Resources[0].Metadata.Labels["label-49"])
}

func TestSet_SpecialCharactersInAnnotationValues(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	annotations := map[string]string{
		"config":  `{"key": "value with \"quotes\"", "nested": {"a": 1}}`,
		"unicode": "日本語テスト",
		"empty":   "",
		"newline": "line1\nline2\nline3",
	}

	resource := makePodResource("uid-special-chars", nil, annotations, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-special-chars")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "日本語テスト", result.Resources[0].Metadata.Annotations["unicode"])
	assert.Equal(t, "line1\nline2\nline3", result.Resources[0].Metadata.Annotations["newline"])
}

// --- Patch edge cases ---

func TestPatch_PreservesOwnerRef(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-patch-owner", map[string]string{"app": "test"}, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	// Patch with only a status change
	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Succeeded"},
	}
	err = repo.Patch(ctx, "uid-patch-owner", partial)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-patch-owner")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	got := result.Resources[0]
	assert.Equal(t, aclmodels.Acl2ScopeCluster, got.RorMeta.Ownerref.Scope,
		"ownerref scope should be preserved after patch")
	assert.Equal(t, aclmodels.Acl2Subject(testClusterID), got.RorMeta.Ownerref.Subject,
		"ownerref subject should be preserved after patch")
}

func TestPatch_PreservesTypeMeta(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-patch-type", nil, nil, "Running")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Failed"},
	}
	err = repo.Patch(ctx, "uid-patch-type", partial)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-patch-type")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Pod", result.Resources[0].Kind, "Kind should be preserved")
	assert.Equal(t, "v1", result.Resources[0].APIVersion, "APIVersion should be preserved")
}

func TestPatch_MultipleSequentialPatches(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-multi-patch", map[string]string{"app": "test"}, nil, "Pending")
	err := repo.Set(ctx, resource)
	require.NoError(t, err)

	// Patch 1: change phase
	partial1 := &rorresources.Resource{}
	partial1.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Running"},
	}
	err = repo.Patch(ctx, "uid-multi-patch", partial1)
	require.NoError(t, err)

	// Patch 2: change phase again with a message
	partial2 := &rorresources.Resource{}
	partial2.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Succeeded", Message: "completed"},
	}
	err = repo.Patch(ctx, "uid-multi-patch", partial2)
	require.NoError(t, err)

	query := rorresources.NewResourceQuery().WithUID("uid-multi-patch")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	got := result.Resources[0]
	assert.Equal(t, "Succeeded", got.PodResource.Status.Phase, "phase from second patch")
	assert.Equal(t, "completed", got.PodResource.Status.Message, "message from second patch")
	assert.Equal(t, "test", got.Metadata.Labels["app"], "labels should survive both patches")
}

func TestPatch_DoesNotAffectOtherResources(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	r1 := makePodResource("uid-iso-1", map[string]string{"app": "one"}, nil, "Running")
	r2 := makePodResource("uid-iso-2", map[string]string{"app": "two"}, nil, "Pending")
	require.NoError(t, repo.Set(ctx, r1))
	require.NoError(t, repo.Set(ctx, r2))

	// Patch only r1
	partial := &rorresources.Resource{}
	partial.PodResource = &rortypes.ResourcePod{
		Status: rortypes.ResourcePodStatus{Phase: "Succeeded"},
	}
	err := repo.Patch(ctx, "uid-iso-1", partial)
	require.NoError(t, err)

	// r2 should be unchanged
	query := rorresources.NewResourceQuery().WithUID("uid-iso-2")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Pending", result.Resources[0].PodResource.Status.Phase,
		"patching r1 should not affect r2")
}

// --- Get / query edge cases ---

func TestGet_MultipleUIDs(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	for i := 0; i < 5; i++ {
		r := makePodResource(fmt.Sprintf("uid-multi-%d", i), nil, nil, "Running")
		r.Metadata.Name = fmt.Sprintf("pod-%d", i)
		require.NoError(t, repo.Set(ctx, r))
	}

	query := rorresources.NewResourceQuery().
		WithUID("uid-multi-0").
		WithUID("uid-multi-2").
		WithUID("uid-multi-4")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resources, 3, "should return exactly the 3 requested UIDs")
}

func TestGet_NonExistentUID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	query := rorresources.NewResourceQuery().WithUID("uid-does-not-exist")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	assert.Nil(t, result, "non-existent UID should return nil")
}

func TestGet_WithFilter(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	r1 := makePodResource("uid-filt-1", nil, nil, "Running")
	r1.Metadata.Name = "web-server"
	r2 := makePodResource("uid-filt-2", nil, nil, "Running")
	r2.Metadata.Name = "db-server"
	r3 := makePodResource("uid-filt-3", nil, nil, "Running")
	r3.Metadata.Name = "web-worker"
	require.NoError(t, repo.Set(ctx, r1))
	require.NoError(t, repo.Set(ctx, r2))
	require.NoError(t, repo.Set(ctx, r3))

	// Get all with regex filter on name
	query := rorresources.NewResourceQuery()
	query.SetLimit(-1)
	query.Filters = []rorresources.ResourceQueryFilter{
		{
			Field:    "metadata.name",
			Value:    "web",
			Type:     rorresources.FilterTypeString,
			Operator: rorresources.FilterOperatorRegexp,
		},
	}
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resources, 2, "regex filter should match 2 web-* pods")
}

func TestGet_WithSorting(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	names := []string{"charlie", "alpha", "bravo"}
	for i, name := range names {
		r := makePodResource(fmt.Sprintf("uid-sort-%d", i), nil, nil, "Running")
		r.Metadata.Name = name
		require.NoError(t, repo.Set(ctx, r))
	}

	query := rorresources.NewResourceQuery()
	query.SetLimit(-1)
	query.Order = []rorresources.ResourceQueryOrder{
		{Field: "metadata.name", Descending: false, Index: 0},
	}
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Resources, 3)
	assert.Equal(t, "alpha", result.Resources[0].GetName())
	assert.Equal(t, "bravo", result.Resources[1].GetName())
	assert.Equal(t, "charlie", result.Resources[2].GetName())
}

func TestGet_WithPagination(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	for i := 0; i < 10; i++ {
		r := makePodResource(fmt.Sprintf("uid-page-%02d", i), nil, nil, "Running")
		r.Metadata.Name = fmt.Sprintf("pod-%02d", i)
		require.NoError(t, repo.Set(ctx, r))
	}

	// Page 1: first 3
	query := rorresources.NewResourceQuery()
	query.SetLimit(3)
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resources, 3)

	// Page 2: skip 3, take 3
	query2 := rorresources.NewResourceQuery()
	query2.SetLimit(3)
	query2.Offset = 3
	result2, err := repo.Get(ctx, query2)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Len(t, result2.Resources, 3)

	// Pages should not overlap
	page1UIDs := make(map[string]bool)
	for _, r := range result.Resources {
		page1UIDs[r.GetUID()] = true
	}
	for _, r := range result2.Resources {
		assert.False(t, page1UIDs[r.GetUID()], "page 2 should not contain UIDs from page 1")
	}
}

// --- Del edge cases ---

func TestDel_ThenSetSameUID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-del-reset", nil, nil, "Running")
	require.NoError(t, repo.Set(ctx, resource))
	require.NoError(t, repo.Del(ctx, resource))

	// Re-create with same UID but different data
	resource2 := makePodResource("uid-del-reset", map[string]string{"app": "v2"}, nil, "Pending")
	err := repo.Set(ctx, resource2)
	require.NoError(t, err, "Set after Del with same UID should succeed")

	query := rorresources.NewResourceQuery().WithUID("uid-del-reset")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Pending", result.Resources[0].PodResource.Status.Phase)
	assert.Equal(t, "v2", result.Resources[0].Metadata.Labels["app"])
}

func TestDel_DoesNotAffectOtherResources(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	r1 := makePodResource("uid-del-iso-1", nil, nil, "Running")
	r2 := makePodResource("uid-del-iso-2", nil, nil, "Pending")
	require.NoError(t, repo.Set(ctx, r1))
	require.NoError(t, repo.Set(ctx, r2))

	require.NoError(t, repo.Del(ctx, r1))

	query := rorresources.NewResourceQuery().WithUID("uid-del-iso-2")
	result, err := repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result, "r2 should still exist after deleting r1")
	assert.Equal(t, "Pending", result.Resources[0].PodResource.Status.Phase)
}

func TestDel_DoubleDelete(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-double-del", nil, nil, "Running")
	require.NoError(t, repo.Set(ctx, resource))
	require.NoError(t, repo.Del(ctx, resource))

	// Second delete should also succeed (idempotent)
	err := repo.Del(ctx, resource)
	require.NoError(t, err, "deleting an already-deleted resource should not error")
}

// --- flattenBsonM unit tests ---

func TestFlattenBsonM_SkipsNilValues(t *testing.T) {
	doc := bson.M{
		"a": "hello",
		"b": nil,
		"c": "world",
	}
	result := bson.M{}
	flattenBsonM("", doc, result)

	assert.Equal(t, "hello", result["a"])
	assert.Equal(t, "world", result["c"])
	_, hasB := result["b"]
	assert.False(t, hasB, "nil values should be skipped")
}

func TestFlattenBsonM_SkipsZeroValues(t *testing.T) {
	doc := bson.M{
		"str":    "",
		"num":    int32(0),
		"num64":  int64(0),
		"flag":   false,
		"filled": "present",
	}
	result := bson.M{}
	flattenBsonM("", doc, result)

	assert.Equal(t, "present", result["filled"])
	assert.Len(t, result, 1, "only non-zero values should be included")
}

func TestFlattenBsonM_StopsAtDottedKeys(t *testing.T) {
	doc := bson.M{
		"labels": bson.M{
			"app":                         "test",
			"app.kubernetes.io/component": "api",
		},
	}
	result := bson.M{}
	flattenBsonM("", doc, result)

	// Should store as a single key, not flatten further
	_, hasLabels := result["labels"]
	assert.True(t, hasLabels, "dotted-key map should be stored as leaf value")
	_, hasFlatLabel := result["labels.app"]
	assert.False(t, hasFlatLabel, "should NOT flatten into dotted-key maps")
}

func TestFlattenBsonM_NestedRecursion(t *testing.T) {
	doc := bson.M{
		"level1": bson.M{
			"level2": bson.M{
				"value": "deep",
			},
		},
	}
	result := bson.M{}
	flattenBsonM("", doc, result)

	assert.Equal(t, "deep", result["level1.level2.value"])
}

func TestFlattenBsonM_WithPrefix(t *testing.T) {
	doc := bson.M{
		"name": "test",
	}
	result := bson.M{}
	flattenBsonM("metadata", doc, result)

	assert.Equal(t, "test", result["metadata.name"])
}

func TestFlattenBsonM_EmptyDoc(t *testing.T) {
	result := bson.M{}
	flattenBsonM("", bson.M{}, result)
	assert.Empty(t, result)
}

func TestFlattenBsonM_HandlesBsonD(t *testing.T) {
	doc := bson.M{
		"status": bson.D{
			{Key: "phase", Value: "Running"},
			{Key: "message", Value: "ok"},
		},
	}
	result := bson.M{}
	flattenBsonM("", doc, result)

	assert.Equal(t, "Running", result["status.phase"])
	assert.Equal(t, "ok", result["status.message"])
}

func TestFlattenBsonM_BsonDWithDottedKeys(t *testing.T) {
	doc := bson.M{
		"annotations": bson.D{
			{Key: "argocd.argoproj.io/tracking-id", Value: "test"},
			{Key: "simple", Value: "value"},
		},
	}
	result := bson.M{}
	flattenBsonM("", doc, result)

	// Should store as leaf since the bson.D has a dotted key
	_, hasAnnotations := result["annotations"]
	assert.True(t, hasAnnotations, "bson.D with dotted keys should be stored as leaf")
}

// --- isZeroValue unit tests ---

func TestIsZeroValue(t *testing.T) {
	tests := []struct {
		name   string
		value  interface{}
		isZero bool
	}{
		{"empty string", "", true},
		{"non-empty string", "hello", false},
		{"zero int32", int32(0), true},
		{"non-zero int32", int32(42), false},
		{"zero int64", int64(0), true},
		{"non-zero int64", int64(100), false},
		{"zero float64", float64(0), true},
		{"non-zero float64", float64(3.14), false},
		{"false bool", false, true},
		{"true bool", true, false},
		{"zero DateTime", bson.DateTime(0), true},
		{"Go zero time DateTime", bson.DateTime(-62135596800000), true},
		{"non-zero DateTime", bson.DateTime(1000000), false},
		{"nil", nil, false}, // nil handled separately in flattenBsonM
		{"bson.M", bson.M{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isZero, isZeroValue(tt.value), "isZeroValue(%v)", tt.value)
		})
	}
}

// --- hasDotKeys unit tests ---

func TestHasDotKeys(t *testing.T) {
	assert.True(t, hasDotKeys(bson.M{"a.b": "v"}))
	assert.True(t, hasDotKeys(bson.M{"simple": "v", "dotted.key": "v2"}))
	assert.False(t, hasDotKeys(bson.M{"simple": "v"}))
	assert.False(t, hasDotKeys(bson.M{}))
}

// --- dToM unit tests ---

func TestDToM(t *testing.T) {
	d := bson.D{
		{Key: "a", Value: 1},
		{Key: "b", Value: "two"},
		{Key: "c", Value: true},
	}
	m := dToM(d)
	assert.Equal(t, 1, m["a"])
	assert.Equal(t, "two", m["b"])
	assert.Equal(t, true, m["c"])
	assert.Len(t, m, 3)
}
