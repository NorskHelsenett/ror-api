package mongodbseeding

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/acl"
	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestSnapshotSeedsMongo(t *testing.T) {
	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		t.Skip("set MONGODB_TEST_URI for real snapshot seeding tests")
	}
	snapshotSeedConfig(t, true, "test-credential", "test-salt")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri).SetBSONOptions(&options.BSONOptions{ObjectIDAsHexString: true}))
	if err != nil {
		t.Fatal(err)
	}
	db := client.Database("snapshot_seed_" + bson.NewObjectID().Hex())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.Drop(ctx); err != nil {
			t.Error(err)
		}
		if err := client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		if err := seedSnapshotAPIKey(ctx, db.Collection("apikeys")); err != nil {
			t.Fatal(err)
		}
		if err := seedSnapshotGrant(ctx, db.Collection("acl")); err != nil {
			t.Fatal(err)
		}
	}
	var key apicontracts.ApiKey
	if err := db.Collection("apikeys").FindOne(ctx, bson.M{"identifier": snapshotServiceID}).Decode(&key); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"apikeys", "acl"} {
		count, err := db.Collection(name).CountDocuments(ctx, bson.M{})
		if err != nil || count != 1 {
			t.Fatalf("%s seed is not idempotent: count %d error %v", name, count, err)
		}
	}
	var grants aclmodels.AclV3List
	cursor, err := db.Collection("acl").Find(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	if err := cursor.All(ctx, &grants); err != nil {
		t.Fatal(err)
	}
	source := seededGrantReader{grants}
	resolver := acl.NewResolver(source)
	groups := []string{string(aclprincipal.Service(snapshotServiceID))}
	allowed, err := resolver.HasAccess(ctx, groups, aclscope.ScopeRor, aclscope.SubjectAcl,
		aclmodels.CapRorAclSnapshot.WithVerb(aclmodels.VerbRead))
	if err != nil || !allowed {
		t.Fatalf("seeded service cannot read snapshots: %v", err)
	}
	admin, err := resolver.HasAccess(ctx, groups, aclscope.ScopeRor, aclscope.SubjectAcl,
		aclmodels.CapRor.WithVerb(aclmodels.VerbCreate))
	if err != nil || admin {
		t.Fatalf("snapshot seeding conferred ACL administration rights: %v", err)
	}
	rorconfig.Set("DEV_ACL_SNAPSHOT_API_KEY", "rotated-test-credential")
	if err := seedSnapshotAPIKey(ctx, db.Collection("apikeys")); err != nil {
		t.Fatal(err)
	}
	var rotated apicontracts.ApiKey
	if err := db.Collection("apikeys").FindOne(ctx, bson.M{"identifier": snapshotServiceID}).Decode(&rotated); err != nil {
		t.Fatal(err)
	}
	if rotated.Hash == key.Hash || rotated.Id != key.Id || !rotated.Created.Equal(key.Created) {
		t.Fatal("owned credential rotation did not preserve record identity")
	}
}

type seededGrantReader struct{ grants aclmodels.AclV3List }

func (s seededGrantReader) GetByGroups(context.Context, []string) (aclmodels.AclV3List, error) {
	return s.grants, nil
}
