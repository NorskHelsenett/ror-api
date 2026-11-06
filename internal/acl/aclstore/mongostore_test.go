package aclstore

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
)

// Keep the driver's experimental mock API confined to this test helper.
// Real MongoDB tests remain responsible for durability/concurrency semantics.
func mockStore(t *testing.T, responses ...bson.D) (*MongoStore, *[]bson.Raw) {
	t.Helper()
	deployment := drivertest.NewMockDeployment(responses...)
	commands := []bson.Raw{}
	opts := options.Client().SetRetryReads(false).SetRetryWrites(false).
		SetBSONOptions(&options.BSONOptions{ObjectIDAsHexString: true}).
		SetMonitor(&event.CommandMonitor{Started: func(_ context.Context, e *event.CommandStartedEvent) {
			commands = append(commands, append(bson.Raw(nil), e.Command...))
		}})
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
	db := client.Database("offline_acl")
	return NewMongoStore(func() *mongo.Database { return db }), &commands
}

func success() bson.D { return bson.D{{Key: "ok", Value: 1}} }

func commandFailure() bson.D {
	return bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: 13}, {Key: "errmsg", Value: "test command denied"}}
}

func writeResult(count int) bson.D {
	return bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: count}, {Key: "nModified", Value: count}}
}

func batch(collection string, id int64, docs ...bson.D) bson.D {
	values := bson.A{}
	for _, doc := range docs {
		values = append(values, doc)
	}
	return bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.D{
		{Key: "id", Value: id}, {Key: "ns", Value: "offline_acl." + collection},
		{Key: "firstBatch", Value: values},
	}}}
}

func storedACL(id bson.ObjectID) bson.D {
	return bson.D{
		{Key: "_id", Value: id}, {Key: "version", Value: 3},
		{Key: "group", Value: "team"}, {Key: "scope", Value: "ror"},
		{Key: "subject", Value: "acl"}, {Key: "access", Value: bson.A{"ror:read"}},
		{Key: "created", Value: time.Unix(1, 0).UTC()}, {Key: "issuedby", Value: "original"},
	}
}

func TestMongoStoreReadCommands(t *testing.T) {
	id := bson.NewObjectID()
	for _, name := range []string{"all", "groups", "scope subject", "id"} {
		t.Run(name, func(t *testing.T) {
			store, commands := mockStore(t, batch("acl", 0, storedACL(id)))
			var rows aclmodels.AclV3List
			var err error
			switch name {
			case "all":
				rows, err = store.GetAll(context.Background())
			case "groups":
				rows, err = store.GetByGroups(context.Background(), []string{"team"})
			case "scope subject":
				rows, err = store.GetByScopeSubject(context.Background(), "ror", "acl")
			case "id":
				var row *aclmodels.AclV3ListItem
				row, err = store.GetById(context.Background(), id.Hex())
				if row != nil {
					rows = append(rows, *row)
				}
			}
			if err != nil || len(rows) != 1 || rows[0].Id != id.Hex() || rows[0].Group != "team" {
				t.Fatalf("read: %#v %v", rows, err)
			}
			filter := (*commands)[0].Lookup("filter").Document()
			switch name {
			case "all":
				if filter.Lookup("version").Type == 0 {
					t.Fatal("version filter missing")
				}
			case "groups":
				if got := filter.Lookup("group").Document().Lookup("$in").Array().Index(0).StringValue(); got != "team" {
					t.Fatalf("unexpected group filter %q", got)
				}
			case "scope subject":
				if filter.Lookup("scope").StringValue() != "ror" || filter.Lookup("subject").StringValue() != "acl" {
					t.Fatal("incorrect scope/subject filter")
				}
			case "id":
				if filter.Lookup("_id").ObjectID() != id {
					t.Fatal("incorrect object ID filter")
				}
			}
		})
	}
}

func TestMongoStoreDecodeCases(t *testing.T) {
	for _, tt := range []struct {
		name string
		doc  bson.D
		err  string
	}{
		{"legacy", bson.D{{Key: "version", Value: 2}, {Key: "group", Value: "legacy"},
			{Key: "access", Value: bson.M{"read": true}}, {Key: "kubernetes", Value: bson.M{"logon": true}}}, ""},
		{"bad version type", bson.D{{Key: "version", Value: "wrong"}}, "entry version"},
		{"unknown version", bson.D{{Key: "version", Value: 99}}, "unknown ACL"},
		{"bad v3 access", bson.D{{Key: "version", Value: 3}, {Key: "access", Value: true}}, "V3 ACL"},
		{"bad v2 access", bson.D{{Key: "version", Value: 2}, {Key: "access", Value: true}}, "V2 ACL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := mockStore(t, batch("acl", 0, tt.doc))
			rows, err := store.GetAll(context.Background())
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("got %v, want %s", err, tt.err)
				}
			} else if err != nil || len(rows) != 1 || len(rows[0].Access) != 2 {
				t.Fatalf("legacy conversion: %#v %v", rows, err)
			}
		})
	}
}

func TestMongoStoreReadFailures(t *testing.T) {
	for _, groups := range []bool{false, true} {
		store, _ := mockStore(t, commandFailure())
		var err error
		if groups {
			_, err = store.GetByGroups(context.Background(), []string{"team"})
		} else {
			_, err = store.GetAll(context.Background())
		}
		if err == nil || !strings.Contains(err.Error(), "query ACL") {
			t.Fatalf("query failure: %v", err)
		}
	}
	store, _ := mockStore(t, batch("acl", 1), commandFailure(), success())
	if _, err := store.GetAll(context.Background()); err == nil || !strings.Contains(err.Error(), "cursor error") {
		t.Fatalf("getMore failure: %v", err)
	}
	store, _ = mockStore(t, batch("acl", 0))
	if row, err := store.GetById(context.Background(), bson.NewObjectID().Hex()); err != nil || row != nil {
		t.Fatalf("missing row: %#v %v", row, err)
	}
}

func TestMongoStoreCreateCommands(t *testing.T) {
	id := bson.NewObjectID()
	store, commands := mockStore(t, writeResult(1), batch("acl", 0, storedACL(id)))
	row, err := store.Create(context.Background(), aclmodels.AclV3ListItem{Id: "caller-id", Version: 2, Group: "team"})
	if err != nil || row.Id != id.Hex() {
		t.Fatalf("create: %#v %v", row, err)
	}
	doc := (*commands)[0].Lookup("documents").Array().Index(0).Document()
	if doc.Lookup("version").Int32() != 3 || doc.Lookup("_id").Type != bson.TypeObjectID || doc.Lookup("created").DateTime() == 0 {
		t.Fatalf("server fields not enforced: %s", doc)
	}
	for _, tt := range []struct {
		name      string
		responses []bson.D
		want      string
	}{
		{"insert fails", []bson.D{commandFailure()}, "insert ACL"},
		{"readback fails", []bson.D{writeResult(1), commandFailure()}, "read back inserted"},
		{"readback missing", []bson.D{writeResult(1), batch("acl", 0)}, "not found on read back"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := mockStore(t, tt.responses...)
			if _, err := store.Create(context.Background(), aclmodels.AclV3ListItem{Created: time.Unix(1, 0)}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
		})
	}
}

func TestMongoStoreUpdateCommands(t *testing.T) {
	id := bson.NewObjectID()
	read := batch("acl", 0, storedACL(id))
	store, commands := mockStore(t, read, writeResult(1), read)
	updated, previous, err := store.Update(context.Background(), id.Hex(), aclmodels.AclV3ListItem{
		Id: "ignored", Version: 2, IssuedBy: "changed", Created: time.Now(),
	})
	if err != nil || updated.Id != id.Hex() || previous.Id != id.Hex() {
		t.Fatalf("update: %#v %#v %v", updated, previous, err)
	}
	replacement := (*commands)[1].Lookup("updates").Array().Index(0).Document().Lookup("u").Document()
	if replacement.Lookup("_id").Type != 0 || replacement.Lookup("version").Int32() != 3 ||
		replacement.Lookup("issuedby").StringValue() != "original" || replacement.Lookup("created").DateTime() != 1000 {
		t.Fatalf("immutable fields changed: %s", replacement)
	}
	for _, tt := range []struct {
		name      string
		responses []bson.D
		want      string
	}{
		{"read fails", []bson.D{commandFailure()}, "read existing"},
		{"missing", []bson.D{batch("acl", 0)}, "not found"},
		{"replace fails", []bson.D{read, commandFailure()}, "update ACL"},
		{"concurrently deleted", []bson.D{read, writeResult(0)}, "not found"},
		{"readback fails", []bson.D{read, writeResult(1), commandFailure()}, "read back updated"},
		{"readback missing", []bson.D{read, writeResult(1), batch("acl", 0)}, "not found on read back"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := mockStore(t, tt.responses...)
			if _, _, err := store.Update(context.Background(), id.Hex(), aclmodels.AclV3ListItem{}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
		})
	}
}

func TestMongoStoreDeleteCommands(t *testing.T) {
	id := bson.NewObjectID()
	read := batch("acl", 0, storedACL(id))
	store, commands := mockStore(t, read, writeResult(1))
	row, err := store.Delete(context.Background(), id.Hex())
	if err != nil || row.Id != id.Hex() {
		t.Fatalf("delete: %#v %v", row, err)
	}
	filter := (*commands)[1].Lookup("deletes").Array().Index(0).Document().Lookup("q").Document()
	if filter.Lookup("_id").ObjectID() != id {
		t.Fatal("delete targeted wrong ID")
	}
	for _, tt := range []struct {
		name      string
		responses []bson.D
		want      string
	}{
		{"read fails", []bson.D{commandFailure()}, "before delete"},
		{"missing", []bson.D{batch("acl", 0)}, "not found"},
		{"delete fails", []bson.D{read, commandFailure()}, "delete ACL"},
		{"concurrently deleted", []bson.D{read, writeResult(0)}, "not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := mockStore(t, tt.responses...)
			if _, err := store.Delete(context.Background(), id.Hex()); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
		})
	}
}

func TestMongoStoreMissingDatabaseAndInvalidID(t *testing.T) {
	for _, missingDB := range []bool{true, false} {
		store := NewMongoStore(func() *mongo.Database { return nil })
		if !missingDB {
			store, _ = mockStore(t)
		}
		want := "invalid id"
		if missingDB {
			want = "not initialized"
			if _, err := store.GetAll(context.Background()); err == nil {
				t.Fatal("read accepted missing database")
			}
			if _, err := store.Create(context.Background(), aclmodels.AclV3ListItem{}); err == nil {
				t.Fatal("create accepted missing database")
			}
		}
		_, readErr := store.GetById(context.Background(), "invalid")
		_, _, updateErr := store.Update(context.Background(), "invalid", aclmodels.AclV3ListItem{})
		_, deleteErr := store.Delete(context.Background(), "invalid")
		for _, err := range []error{readErr, updateErr, deleteErr} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("got %v, want %s", err, want)
			}
		}
	}
}
