package seeding

import (
	"context"
	"testing"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
)

func snapshotSeedConfig(t *testing.T, development bool) {
	t.Helper()
	old := rorconfig.GetString(rorconfig.DEVELOPMENT)
	rorconfig.Set(string(rorconfig.DEVELOPMENT), development)
	t.Cleanup(func() { rorconfig.Set(string(rorconfig.DEVELOPMENT), old) })
}

func TestSnapshotGrantSeedSkippedOutsideDevelopment(t *testing.T) {
	snapshotSeedConfig(t, false)
	if err := seedSnapshotGrant(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotGrantSeed(t *testing.T) {
	snapshotSeedConfig(t, true)
	for _, fail := range []bool{false, true} {
		response := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 0}}
		if fail {
			response = bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: 13}, {Key: "errmsg", Value: "denied"}}
		}
		deployment := drivertest.NewMockDeployment(response)
		var command bson.Raw
		opts := options.Client().SetRetryWrites(false).SetMonitor(&event.CommandMonitor{
			Started: func(_ context.Context, e *event.CommandStartedEvent) { command = append(bson.Raw(nil), e.Command...) },
		})
		opts.Deployment = deployment
		client, err := mongo.Connect(opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := client.Disconnect(context.Background()); err != nil {
				t.Error(err)
			}
		})
		err = seedSnapshotGrant(context.Background(), client.Database("seed").Collection("acl"))
		if (err != nil) != fail {
			t.Fatalf("seed grant: %v", err)
		}
		op := command.Lookup("updates").Array().Index(0).Document()
		grant := op.Lookup("u").Document().Lookup("$setOnInsert").Document()
		if !op.Lookup("upsert").Boolean() || grant.Lookup("scope").StringValue() != "ror" ||
			grant.Lookup("subject").StringValue() != "acl" ||
			grant.Lookup("group").StringValue() != string(aclprincipal.Service(SnapshotServiceID)) {
			t.Fatal("snapshot grant was not seeded for the fixed development service")
		}
		access := grant.Lookup("access").Array()
		values, err := access.Values()
		if err != nil || len(values) != 1 || values[0].StringValue() != "ror:acl:snapshot:read" {
			t.Fatal("snapshot seed granted unrelated privileges")
		}
		if err := aclmodels.ValidateAccess(aclmodels.AccessTypeV3(values[0].StringValue())); err != nil {
			t.Fatal(err)
		}
		if op.Lookup("u").Document().Lookup("$set").Type != 0 {
			t.Fatal("grant seeding must not overwrite existing grants")
		}
	}
}
