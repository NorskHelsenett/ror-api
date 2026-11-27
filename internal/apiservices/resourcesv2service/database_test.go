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

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/internal/mocks/identitymocks"
	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/helpers/optional"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/rorresources/rortypes"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
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

// testConfigWriterGroup holds a V3 grant with the protected Config write
// capability, so protected-kind write enforcement can be exercised for a grant
// holder (any identity type), not only the denied cluster.
const testConfigWriterGroup = "config-writers@e2e.invalid"

// testAclStore grants the test cluster access to its own resources. Cluster
// access is ordinary ACL data, so without it every query here is denied.
type testAclStore struct{}

func (testAclStore) GetByGroups(_ context.Context, groups []string) (aclmodels.AclV3List, error) {
	var out aclmodels.AclV3List
	for _, g := range groups {
		switch g {
		case aclprincipal.Cluster(testClusterID):
			out = append(out, aclmodels.AclV3ListItem{
				Version: 3,
				Group:   g,
				Scope:   aclscope.ScopeCluster,
				Subject: aclscope.Subject(testClusterID),
				Access:  aclservice.ClusterSelfAccess(),
			})
		case testConfigWriterGroup:
			out = append(out, aclmodels.AclV3ListItem{
				Version: 3,
				Group:   g,
				Scope:   aclscope.ScopeRor,
				Subject: aclscope.Subject("Config"),
				Access: []aclmodels.AccessTypeV3{
					aclmodels.CapRorConfig.WithVerb(aclmodels.VerbRead),
					aclmodels.CapRorConfig.WithVerb(aclmodels.VerbWrite),
				},
			})
		}
	}
	return out, nil
}

// TestMain wires the ACL resolver once for the package: the database layer
// always applies an ACL filter, so the tests need a resolver to query through.
func TestMain(m *testing.M) {
	aclservice.SetResolver(acl.NewResolver(testAclStore{}))
	os.Exit(m.Run())
}

// testCtx returns a context with a cluster identity matching testClusterID.
func testCtx() context.Context {
	identity := identitymocks.Cluster(testClusterID, testClusterID)
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
			Scope:   aclscope.ScopeCluster,
			Subject: aclscope.Subject(testClusterID),
		},
	}
	return r
}

// TODO: Fix commented out tests for patch
func makePatchResource(uid string, kind string, labels map[string]string, annotations map[string]string) *rorresources.ResourcePatch {
	r := &rorresources.ResourcePatch{}
	r.TypeMeta = metav1.TypeMeta{Kind: kind, APIVersion: "v1"}
	r.UID = uid

	for key, value := range labels {
		r.SetLabel(key, value)
	}

	for key, value := range annotations {
		r.SetAnnotation(key, value)
	}

	rorMeta := rortypes.ResourcePatchRorMeta{
		Version: optional.NewOptional("v2"),
		Ownerref: optional.NewOptional(rorresourceowner.RorResourceOwnerReference{
			Scope:   aclscope.ScopeCluster,
			Subject: aclscope.Subject(testClusterID),
		}),
	}
	r.RorMeta = optional.NewOptional(rorMeta)
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

	// Step 3: Patch the labels by applying the patch and writing the result back
	patch := makePatchResource("uid-roundtrip", "Pod", nil, nil)
	patch.RorMeta.Unset()
	patch.SetLabel("phase", "running")
	patched, err := patch.Apply(got)
	require.NoError(t, err)
	patched.GenRorHash()
	require.NoError(t, repo.Set(ctx, patched))

	// Step 4: Get and verify patch preserved labels/annotations
	result, err = repo.Get(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, result)
	got = result.Resources[0]
	assert.Equal(t, "running", got.Metadata.Labels["phase"], "label should be added")
	assert.Equal(t, "Pending", got.PodResource.Status.Phase, "untouched fields should survive patch")
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

func TestSet_Idempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	resource := makePodResource("uid-idem", map[string]string{"app": "test"}, nil, "Running")

	// Set the same resource 3 times — should always succeed with the same data
	for i := range 3 {
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
	resource.APIVersion = "apps/v1"
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
	for i := range 50 {
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

// --- Get / query edge cases ---

func TestGet_MultipleUIDs(t *testing.T) {
	repo := newTestRepo(t)
	ctx := testCtx()

	for i := range 5 {
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

	for i := range 10 {
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
