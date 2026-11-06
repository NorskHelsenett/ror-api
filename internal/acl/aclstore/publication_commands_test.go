package aclstore

import (
	"context"
	"strings"
	"testing"
	"time"

	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPublishSnapshotRetriesStaleObservation(t *testing.T) {
	limits := publicationLimits()
	current, err := nextPublication(publication{Revision: "0"}, []snapshot.Row{policyRow("team")}, limits, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	current.Observation = bson.NewObjectID()
	raw, err := bson.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	var persisted bson.D
	if err := bson.Unmarshal(raw, &persisted); err != nil {
		t.Fatal(err)
	}
	initial := batch(publicationCollection, 0, bson.D{{Key: "revision", Value: "0"}})
	// A stale scan saw no rows. Another publisher won the CAS with "team".
	// Retry must reread the publication and rescan, never publish the stale scan.
	store, commands := mockStore(t, writeResult(1),
		initial, batch("acl", 0), writeResult(0),
		batch(publicationCollection, 0, persisted), batch("acl", 0, storedACL(bson.NewObjectID())), writeResult(1))
	payload, err := store.PublishSnapshot(context.Background(), limits)
	if err != nil || string(payload) != string(current.Payload) {
		t.Fatalf("stale publisher did not return the rescanned unchanged generation: %v", err)
	}
	if len(*commands) != 7 {
		t.Fatalf("expected full reread and rescan after conflict, got %d commands", len(*commands))
	}
	op := (*commands)[6].Lookup("updates").Array().Index(0).Document()
	if op.Lookup("q").Document().Lookup("observation").ObjectID() != current.Observation {
		t.Fatal("retry did not compare the observation token")
	}
	token := op.Lookup("u").Document().Lookup("$set").Document().Lookup("observation").ObjectID()
	if token.IsZero() || token == current.Observation {
		t.Fatal("unchanged publication did not replace its observation token")
	}
}

func TestPublishSnapshotConcurrentInitializationDuplicate(t *testing.T) {
	duplicate := bson.D{{Key: "ok", Value: 0}, {Key: "code", Value: 11000}, {Key: "errmsg", Value: "duplicate _id"}}
	initial := batch(publicationCollection, 0, bson.D{{Key: "revision", Value: "0"}})
	store, _ := mockStore(t, duplicate, initial, batch("acl", 0), writeResult(1))
	if _, err := store.PublishSnapshot(context.Background(), publicationLimits()); err != nil {
		t.Fatalf("concurrent initializer was not accepted: %v", err)
	}
}

func TestPublishSnapshotContentionBound(t *testing.T) {
	responses := []bson.D{writeResult(1)}
	for range 32 {
		responses = append(responses,
			batch(publicationCollection, 0, bson.D{{Key: "revision", Value: "0"}}),
			batch("acl", 0), writeResult(0))
	}
	store, commands := mockStore(t, responses...)
	payload, err := store.PublishSnapshot(context.Background(), publicationLimits())
	if err == nil || payload != nil || !strings.Contains(err.Error(), "contention exceeded") {
		t.Fatalf("expected explicit bounded contention failure, got %s %v", payload, err)
	}
	if len(*commands) != 97 {
		t.Fatalf("expected initialization plus 32 bounded attempts, got %d commands", len(*commands))
	}
}

func TestPublishSnapshotCommands(t *testing.T) {
	initial := batch(publicationCollection, 0, bson.D{{Key: "revision", Value: "0"}})
	acl := batch("acl", 0, storedACL(bson.NewObjectID()))
	store, commands := mockStore(t, writeResult(1), initial, acl, writeResult(1))
	payload, err := store.PublishSnapshot(context.Background(), publicationLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := snapshot.Decode(payload, publicationLimits())
	if err != nil || p.Revision != 1 || len(p.Rows) != 1 || p.Rows[0].Group != "team" {
		t.Fatalf("published: %#v %v", p, err)
	}
	if len(*commands) != 4 {
		t.Fatalf("got %d commands, want initialize/read/scan/compare-and-swap", len(*commands))
	}
	for _, command := range *commands {
		if command.Lookup("startTransaction").Type != 0 || command.Lookup("commitTransaction").Type != 0 {
			t.Fatal("standalone publication must not use transactions")
		}
	}
	scan := (*commands)[2]
	if scan.Lookup("find").StringValue() != "acl" || scan.Lookup("limit").Int64() != 11 ||
		scan.Lookup("projection").Document().Lookup("kubernetes").Int32() != 1 {
		t.Fatalf("authoritative scan is not bounded or loses legacy grants: %s", scan)
	}
	op := (*commands)[3].Lookup("updates").Array().Index(0).Document()
	condition := op.Lookup("q").Document()
	if condition.Lookup("_id").StringValue() != publicationID ||
		condition.Lookup("observation").Document().Lookup("$exists").Boolean() {
		t.Fatal("initial publication compare-and-swap condition missing")
	}
	update := op.Lookup("u").Document()
	saved := update.Lookup("$set").Document()
	if saved.Lookup("revision").StringValue() != "1" {
		t.Fatal("wrong durable revision")
	}
	_, savedPayload := saved.Lookup("payload").Binary()
	if string(savedPayload) != string(payload) {
		t.Fatal("response differs from persisted publication")
	}
	if saved.Lookup("observation").ObjectID().IsZero() {
		t.Fatal("publication observation token missing")
	}
}

func TestPublishSnapshotCommandFailures(t *testing.T) {
	initial := batch(publicationCollection, 0, bson.D{{Key: "revision", Value: "0"}})
	empty := batch("acl", 0)
	for _, tt := range []struct {
		name      string
		responses []bson.D
		want      string
	}{
		{"initialization", []bson.D{commandFailure()}, "initialize ACL"},
		{"publication read", []bson.D{writeResult(1), commandFailure()}, "read ACL publication"},
		{"missing publication", []bson.D{writeResult(1), batch(publicationCollection, 0)}, "read ACL publication"},
		{"authoritative read", []bson.D{writeResult(1), initial, commandFailure()}, "authoritative ACL"},
		{"corrupt revision", []bson.D{writeResult(1),
			batch(publicationCollection, 0, bson.D{{Key: "revision", Value: "invalid"}}), empty}, "revision is corrupt"},
		{"persist", []bson.D{writeResult(1), initial, empty, commandFailure()}, "persist ACL"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := mockStore(t, tt.responses...)
			payload, err := store.PublishSnapshot(context.Background(), publicationLimits())
			if err == nil || payload != nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got (%s, %v), want no payload and %s", payload, err, tt.want)
			}
		})
	}
}

func TestSnapshotRowsCommands(t *testing.T) {
	legacy := bson.D{{Key: "version", Value: 2}, {Key: "group", Value: "legacy"},
		{Key: "scope", Value: "ror"}, {Key: "subject", Value: "acl"}, {Key: "access", Value: bson.M{}}}
	store, _ := mockStore(t, batch("acl", 0, legacy))
	rows, err := snapshotRows(context.Background(), store.dbProvider(), publicationLimits())
	if err != nil || len(rows) != 1 || rows[0].Grant.Access == nil || len(rows[0].Grant.Access) != 0 {
		t.Fatalf("legacy inert row: %#v %v", rows, err)
	}
	for _, tt := range []struct {
		name      string
		responses []bson.D
		limits    snapshot.Limits
		want      string
	}{
		{"query", []bson.D{commandFailure()}, publicationLimits(), "authoritative ACL rows"},
		{"decode", []bson.D{batch("acl", 0, bson.D{{Key: "version", Value: 99}})}, publicationLimits(), "unknown ACL"},
		{"cursor", []bson.D{batch("acl", 1), commandFailure(), success()}, publicationLimits(), "authoritative ACL cursor"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, _ := mockStore(t, tt.responses...)
			if rows, err := snapshotRows(context.Background(), store.dbProvider(), tt.limits); err == nil || rows != nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got (%#v, %v), want %s", rows, err, tt.want)
			}
		})
	}
	limits := publicationLimits()
	limits.MaxRows = 1
	store, _ = mockStore(t, batch("acl", 0, legacy, legacy))
	if _, err := snapshotRows(context.Background(), store.dbProvider(), limits); err == nil || !strings.Contains(err.Error(), "row bound") {
		t.Fatalf("row bound: %v", err)
	}
	limits = publicationLimits()
	limits.MaxPayloadBytes = 10
	store, _ = mockStore(t, batch("acl", 0, legacy))
	if _, err := snapshotRows(context.Background(), store.dbProvider(), limits); err == nil || !strings.Contains(err.Error(), "byte bound") {
		t.Fatalf("byte bound: %v", err)
	}
}

func TestFindCursorCloseFailure(t *testing.T) {
	// A malformed row terminates iteration early and closes the live cursor.
	// Failure to close must not turn the decoding error into a successful read.
	store, _ := mockStore(t, batch("acl", 1, bson.D{{Key: "version", Value: 99}}), commandFailure())
	if _, err := store.GetAll(context.Background()); err == nil || !strings.Contains(err.Error(), "unknown ACL") {
		t.Fatalf("decoding error lost during close failure: %v", err)
	}
	store, _ = mockStore(t, batch("acl", 1, bson.D{{Key: "version", Value: 99}}), commandFailure())
	if _, err := snapshotRows(context.Background(), store.dbProvider(), publicationLimits()); err == nil || !strings.Contains(err.Error(), "unknown ACL") {
		t.Fatalf("snapshot decoding error lost during close failure: %v", err)
	}
}
