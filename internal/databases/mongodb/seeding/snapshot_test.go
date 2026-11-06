package mongodbseeding

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

func snapshotSeedConfig(t *testing.T, development bool, credential, salt string) {
	t.Helper()
	values := map[string]string{
		"DEVELOPMENT":                      "false",
		"DEV_ACL_SNAPSHOT_API_KEY":         credential,
		string(rorconfig.ROR_API_KEY_SALT): salt,
	}
	if development {
		values["DEVELOPMENT"] = "true"
	}
	for key, value := range values {
		old := rorconfig.GetString(key)
		rorconfig.Set(key, value)
		t.Cleanup(func() { rorconfig.Set(key, old) })
	}
}

func TestSnapshotSeedGating(t *testing.T) {
	for _, tt := range []struct {
		name        string
		development bool
		credential  string
		salt        string
		wantError   bool
	}{
		{"production", false, "test-credential", "test-salt", false},
		{"disabled", true, "", "", false},
		{"missing salt", true, "test-credential", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshotSeedConfig(t, tt.development, tt.credential, tt.salt)
			err := seedSnapshotAPIKey(context.Background(), nil)
			if (err != nil) != tt.wantError {
				t.Fatalf("seed error: %v", err)
			}
			if !tt.development || tt.credential == "" {
				if err := seedSnapshotGrant(context.Background(), nil); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestSnapshotGrantSeed(t *testing.T) {
	snapshotSeedConfig(t, true, "test-credential", "test-salt")
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
			grant.Lookup("group").StringValue() != string(aclprincipal.Service(snapshotServiceID)) {
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

func TestSnapshotSeedCommands(t *testing.T) {
	failure := bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: 13}, {Key: "errmsg", Value: "denied"}}
	success := bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}, {Key: "nModified", Value: 1}}
	for _, tt := range []struct {
		name       string
		docs       bson.A
		findError  bool
		writeError bool
		lostOwner  bool
		wantError  bool
		wantWrite  bool
	}{
		{name: "insert", docs: bson.A{}, wantWrite: true},
		{name: "existing", docs: bson.A{bson.M{"_id": "owned-id", "type": "Service", "seedOwner": snapshotSeedOwner}}, wantWrite: true},
		{name: "unrelated service", docs: bson.A{bson.M{"type": "Service"}}, wantError: true},
		{name: "different owner", docs: bson.A{bson.M{"type": "Service", "seedOwner": "other"}}, wantError: true},
		{name: "ownership changed", docs: bson.A{bson.M{"_id": "owned-id", "type": "Service", "seedOwner": snapshotSeedOwner}}, lostOwner: true, wantWrite: true, wantError: true},
		{name: "wrong type", docs: bson.A{bson.M{"type": "User"}}, wantError: true},
		{name: "duplicate", docs: bson.A{bson.M{"type": "Service"}, bson.M{"type": "Service"}}, wantError: true},
		{name: "decode error", docs: bson.A{bson.M{"type": bson.M{"invalid": true}}}, wantError: true},
		{name: "find error", findError: true, wantError: true},
		{name: "write error", docs: bson.A{}, writeError: true, wantWrite: true, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshotSeedConfig(t, true, "test-credential", "test-salt")
			find := bson.D{{Key: "ok", Value: 1}, {Key: "cursor", Value: bson.M{
				"id": int64(0), "ns": "seed.apikeys", "firstBatch": tt.docs,
			}}}
			if tt.findError {
				find = failure
			}
			write := success
			if tt.writeError {
				write = failure
			}
			if tt.lostOwner {
				write = bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 0}, {Key: "nModified", Value: 0}}
			}
			deployment := drivertest.NewMockDeployment(find, write)
			var commands []bson.Raw
			opts := options.Client().SetRetryReads(false).SetRetryWrites(false).
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
			err = seedSnapshotAPIKey(context.Background(), client.Database("seed").Collection("apikeys"))
			if (err != nil) != tt.wantError {
				t.Fatalf("seed error: %v", err)
			}
			if !tt.wantWrite {
				if len(commands) != 1 {
					t.Fatal("invalid seed performed a write")
				}
				return
			}
			if len(commands) != 2 {
				t.Fatalf("expected find and update, got %d commands", len(commands))
			}
			op := commands[1].Lookup("updates").Array().Index(0).Document()
			query := op.Lookup("q").Document()
			if op.Lookup("upsert").Boolean() != (len(tt.docs) == 0) ||
				query.Lookup("identifier").StringValue() != "acl-snapshot" ||
				query.Lookup("type").StringValue() != "Service" ||
				query.Lookup("seedOwner").StringValue() != snapshotSeedOwner {
				t.Fatal("seed write must guard ownership and only upsert new records")
			}
			wantID := "owned-id"
			if len(tt.docs) == 0 {
				wantID = snapshotSeedOwner + ":acl-snapshot"
			}
			if query.Lookup("_id").StringValue() != wantID {
				t.Fatal("seed write targets the wrong credential")
			}
			update := op.Lookup("u").Document()
			set := update.Lookup("$set").Document()
			digest := sha512.Sum512([]byte("test-credentialtest-salt"))
			if set.Lookup("hash").StringValue() != hex.EncodeToString(digest[:]) {
				t.Fatal("credential was not hashed with the API key salt")
			}
			if set.Lookup("type").StringValue() != "Service" || !set.Lookup("readonly").Boolean() {
				t.Fatal("seed must be a read-only service credential")
			}
			if update.Lookup("$setOnInsert").Document().Lookup("created").Type == 0 || set.Lookup("created").Type != 0 {
				t.Fatal("seed must preserve the original creation time")
			}
		})
	}
}

func TestLocalSnapshotEnvironment(t *testing.T) {
	script, err := filepath.Abs("../../../../hacks/assets/vault/local-snapshot-env.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, initial := range []string{
		"UNRELATED=keep\n",
		"UNRELATED=keep\nDEV_ACL_SNAPSHOT_API_KEY=\"\"\n",
		"UNRELATED=keep\nDEV_ACL_SNAPSHOT_API_KEY=''\n",
		"UNRELATED=keep\nACL_SNAPSHOT_MAX_ROWS=42\nDEV_ACL_SNAPSHOT_API_KEY=existing-local-fixture\n",
	} {
		t.Run(initial, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(initial), 0600); err != nil {
				t.Fatal(err)
			}
			run := func() string {
				t.Helper()
				out, err := exec.Command("sh", script, path).CombinedOutput()
				if err != nil {
					t.Fatalf("bootstrap: %v: %s", err, out)
				}
				if len(out) != 0 {
					t.Fatal("bootstrap must not print credentials")
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			first := run()
			if !strings.Contains(initial, "ACL_SNAPSHOT_MAX_") && strings.Contains(first, "ACL_SNAPSHOT_MAX_") {
				t.Fatal("bootstrap added unnecessary snapshot limit configuration")
			}
			if !strings.Contains(first, "UNRELATED=keep\n") {
				t.Fatal("missing local configuration or changed unrelated configuration")
			}
			if strings.Contains(initial, "existing-local-fixture") {
				if !strings.Contains(first, "ACL_SNAPSHOT_MAX_ROWS=42\n") ||
					!strings.Contains(first, "DEV_ACL_SNAPSHOT_API_KEY=existing-local-fixture\n") {
					t.Fatal("bootstrap overwrote local overrides")
				}
			} else if !regexp.MustCompile(`(?m)^DEV_ACL_SNAPSHOT_API_KEY="[0-9a-f]{64}"$`).MatchString(first) {
				t.Fatal("expected a generated 256-bit development credential")
			}
			if second := run(); second != first {
				t.Fatal("restart changed the development key or configuration")
			}
		})
	}
}
