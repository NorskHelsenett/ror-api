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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/x/mongo/driver/drivertest"
)

func snapshotSeedConfig(t *testing.T, development bool, credential, identifier, salt string) {
	t.Helper()
	values := map[string]string{
		"DEVELOPMENT":                      "false",
		"DEV_ACL_SNAPSHOT_API_KEY":         credential,
		"ACL_SNAPSHOT_READER_SERVICE_ID":   identifier,
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
		identifier  string
		salt        string
		wantError   bool
	}{
		{"production", false, "test-credential", "acl-snapshot", "test-salt", false},
		{"disabled", true, "", "", "", false},
		{"missing key", true, "", "acl-snapshot", "test-salt", true},
		{"missing identifier", true, "test-credential", "", "test-salt", true},
		{"missing salt", true, "test-credential", "acl-snapshot", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshotSeedConfig(t, tt.development, tt.credential, tt.identifier, tt.salt)
			err := seedSnapshotAPIKey(context.Background(), nil)
			if (err != nil) != tt.wantError {
				t.Fatalf("seed error: %v", err)
			}
		})
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
		wantError  bool
		wantWrite  bool
	}{
		{name: "insert", docs: bson.A{}, wantWrite: true},
		{name: "existing", docs: bson.A{bson.M{"type": "Service"}}, wantWrite: true},
		{name: "wrong type", docs: bson.A{bson.M{"type": "User"}}, wantError: true},
		{name: "duplicate", docs: bson.A{bson.M{"type": "Service"}, bson.M{"type": "Service"}}, wantError: true},
		{name: "decode error", docs: bson.A{bson.M{"type": bson.M{"invalid": true}}}, wantError: true},
		{name: "find error", findError: true, wantError: true},
		{name: "write error", docs: bson.A{}, writeError: true, wantWrite: true, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			snapshotSeedConfig(t, true, "test-credential", "acl-snapshot", "test-salt")
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
			if !op.Lookup("upsert").Boolean() || op.Lookup("q").Document().Lookup("identifier").StringValue() != "acl-snapshot" {
				t.Fatal("seed must upsert by service identifier")
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
		"UNRELATED=keep\nACL_SNAPSHOT_MAX_ROWS=42\nACL_SNAPSHOT_READER_SERVICE_ID=custom\nDEV_ACL_SNAPSHOT_API_KEY=existing-local-fixture\n",
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
			if !strings.Contains(first, "UNRELATED=keep\n") {
				t.Fatal("missing local configuration or changed unrelated configuration")
			}
			if strings.Contains(initial, "existing-local-fixture") {
				if !strings.Contains(first, "ACL_SNAPSHOT_MAX_ROWS=42\n") ||
					!strings.Contains(first, "ACL_SNAPSHOT_READER_SERVICE_ID=custom\n") ||
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
