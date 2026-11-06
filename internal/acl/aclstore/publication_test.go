package aclstore

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclgrants"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func publicationLimits() snapshot.Limits {
	return snapshot.Limits{MaxPayloadBytes: 8192, MaxEnvelopeBytes: 16384, MaxRows: 10,
		MaxAccessEntries: 10, MaxFieldBytes: 128, MaxHeaderBytes: 256}
}

// An explicit URI makes missing/broken infrastructure a failure, not a skip.
func newPublicationTestDB(t *testing.T) (*mongo.Database, *MongoStore) {
	t.Helper()
	if os.Getenv("MONGODB_TEST_URI") == "" {
		t.Skip("set MONGODB_TEST_URI to MongoDB; see docker-compose.test.yml")
	}
	return newStoreTestDB(t)
}

func policyRow(group string) snapshot.Row {
	return snapshot.Row{Group: group, Grant: aclgrants.Grant{
		Scope: "ror", Subject: "acl", Access: []aclmodels.AccessTypeV3{"ror:read"},
	}}
}

func TestNextPublication(t *testing.T) {
	limits := publicationLimits()
	now := time.Unix(1, 0).UTC()
	current, err := nextPublication(publication{Revision: "0"}, []snapshot.Row{}, limits, now)
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != "1" || !strings.Contains(string(current.Payload), `"rows":[]`) {
		t.Fatalf("invalid initial empty policy: %#v", current)
	}
	same, err := nextPublication(current, []snapshot.Row{}, limits, now.Add(time.Hour))
	if err != nil || string(same.Payload) != string(current.Payload) {
		t.Fatalf("unchanged generation: %v", err)
	}
	next, err := nextPublication(current, []snapshot.Row{policyRow("service@ror.io")}, limits, now.Add(time.Hour))
	if err != nil || next.Revision != "2" {
		t.Fatalf("changed generation: %#v %v", next, err)
	}
	smallerBounds := limits
	smallerBounds.MaxPayloadBytes = len(current.Payload)
	smallerBounds.MaxFieldBytes = 1
	smaller, err := nextPublication(next, []snapshot.Row{}, smallerBounds, now.Add(2*time.Hour))
	if err != nil || smaller.Revision != "3" {
		t.Fatalf("tighter bounds prevented a smaller valid policy: %#v %v", smaller, err)
	}
	smallerBounds.MaxFieldBytes = limits.MaxFieldBytes
	if _, err := nextPublication(next, []snapshot.Row{policyRow("service@ror.io")}, smallerBounds, now); err == nil {
		t.Fatal("unchanged publication bypassed tightened payload bound")
	}
	reverted, err := nextPublication(next, []snapshot.Row{}, limits, now.Add(2*time.Hour))
	if err != nil || reverted.Revision != "3" {
		t.Fatalf("reverted generation: %#v %v", reverted, err)
	}
	for _, corrupt := range []publication{
		{Revision: "-1"}, {Revision: "01"}, {Revision: "0", ContentHash: "unexpected"},
		{Revision: "1", Payload: []byte("{}")},
		{Revision: "2", Payload: current.Payload, ContentHash: current.ContentHash},
		{Revision: "1", Payload: current.Payload, ContentHash: "wrong"},
	} {
		if _, err := nextPublication(corrupt, []snapshot.Row{}, limits, now); err == nil {
			t.Fatalf("accepted corrupt publication %#v", corrupt)
		}
	}
	maxPayload, err := snapshot.Encode(snapshot.Payload{
		Schema: snapshot.Schema, Revision: snapshot.Revision(math.MaxUint64),
		GeneratedAt: now, Rows: []snapshot.Row{},
	}, limits)
	if err != nil {
		t.Fatal(err)
	}
	exhausted := current
	exhausted.Revision = fmt.Sprint(uint64(math.MaxUint64))
	exhausted.Payload = maxPayload
	if _, err := nextPublication(exhausted, []snapshot.Row{policyRow("new")}, limits, now); err == nil {
		t.Fatal("revision wrapped")
	}
	if _, err := nextPublication(publication{Revision: "0"}, nil, limits, now); err == nil {
		t.Fatal("invalid source accepted")
	}
	limits.MaxPayloadBytes = 1
	if _, err := nextPublication(publication{Revision: "0"}, []snapshot.Row{}, limits, now); err == nil {
		t.Fatal("oversized generation accepted")
	}
	store := NewMongoStore(func() *mongo.Database { return nil })
	if _, err := store.PublishSnapshot(context.Background(), publicationLimits()); err == nil {
		t.Fatal("missing database accepted")
	}
	if _, err := store.PublishSnapshot(context.Background(), snapshot.Limits{}); err == nil {
		t.Fatal("missing limits accepted")
	}
}

func TestRepeatedRowsPublication(t *testing.T) {
	rows := []snapshot.Row{policyRow("team"), policyRow("team"), policyRow("team")}
	rows[1].Grant.Access = []aclmodels.AccessTypeV3{"ror:write", "ror:read"}
	now := time.Unix(1, 0).UTC()
	first, err := nextPublication(publication{Revision: "0"}, rows, publicationLimits(), now)
	if err != nil {
		t.Fatal(err)
	}
	p, err := snapshot.Decode(first.Payload, publicationLimits())
	if err != nil || len(p.Rows) != len(rows) {
		t.Fatalf("repeated rows not preserved: %#v %v", p, err)
	}
	rows[0], rows[1] = rows[1], rows[0]
	rows[0].Grant.Access = []aclmodels.AccessTypeV3{"ror:read", "ror:write"}
	same, err := nextPublication(first, rows, publicationLimits(), now.Add(time.Hour))
	if err != nil || string(same.Payload) != string(first.Payload) || same.ContentHash != first.ContentHash {
		t.Fatalf("reordered repeated rows changed publication: %v", err)
	}
	removed, err := nextPublication(first, rows[:2], publicationLimits(), now.Add(time.Hour))
	if err != nil || removed.Revision != "2" {
		t.Fatalf("removing an identical repetition was not observed: %#v %v", removed, err)
	}
}

func TestPublishSnapshotMongo(t *testing.T) {
	db, store := newPublicationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	limits := publicationLimits()
	initial, err := store.PublishSnapshot(ctx, limits)
	if err != nil {
		t.Fatal(err)
	}
	p, err := snapshot.Decode(initial, limits)
	if err != nil || p.Revision != 1 || len(p.Rows) != 0 {
		t.Fatalf("initial policy: %#v %v", p, err)
	}
	row := policyRow("service@ror.io")
	_, err = db.Collection("acl").InsertOne(ctx, bson.M{
		"version": 3, "group": row.Group, "scope": row.Grant.Scope,
		"subject": row.Grant.Subject, "access": row.Grant.Access,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.PublishSnapshot(ctx, limits)
	if err != nil {
		t.Fatal(err)
	}
	p, err = snapshot.Decode(updated, limits)
	if err != nil || p.Revision != 2 || len(p.Rows) != 1 || p.Rows[0].Group != row.Group {
		t.Fatalf("updated policy: %#v %v", p, err)
	}
	if _, err := db.Collection("acl").UpdateMany(ctx, bson.M{}, bson.M{"$set": bson.M{"issuedBy": "metadata-only"}}); err != nil {
		t.Fatal(err)
	}
	restarted := NewMongoStore(func() *mongo.Database { return db })
	stable, err := restarted.PublishSnapshot(ctx, limits)
	if err != nil || string(stable) != string(updated) {
		t.Fatalf("restart changed publication: %v", err)
	}
	_, err = db.Collection("acl").DeleteMany(ctx, bson.M{})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.PublishSnapshot(ctx, limits)
	if err != nil {
		t.Fatal(err)
	}
	p, _ = snapshot.Decode(empty, limits)
	if p.Revision != 3 || len(p.Rows) != 0 {
		t.Fatal("direct deletion did not advance policy")
	}
	if _, err := db.Collection(publicationCollection).UpdateOne(ctx, bson.M{"_id": publicationID},
		bson.M{"$set": bson.M{"payload": []byte("corrupt")}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishSnapshot(ctx, limits); err == nil {
		t.Fatal("corrupt publication accepted")
	}
}

func TestPublishSnapshotConcurrentInitializationMongo(t *testing.T) {
	_, store := newPublicationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := store.PublishSnapshot(ctx, publicationLimits())
			if err != nil {
				errs <- err
				return
			}
			p, err := snapshot.Decode(data, publicationLimits())
			if err != nil {
				errs <- err
				return
			}
			if p.Revision != 1 {
				errs <- fmt.Errorf("initial revision %d, want 1", p.Revision)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestPublishSnapshotConcurrentMongo(t *testing.T) {
	db, store := newPublicationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	limits := publicationLimits()
	if _, err := store.PublishSnapshot(ctx, limits); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan []byte, 12)
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := db.Collection("acl").InsertOne(ctx, bson.M{
				"version": 3, "group": fmt.Sprintf("g-%d", i), "scope": "ror",
				"subject": "acl", "access": bson.A{"ror:read"},
			})
			if err != nil {
				errs <- err
				return
			}
			// This test deliberately uses a larger bound than the unit fixtures.
			bound := limits
			bound.MaxRows = 20
			payload, err := store.PublishSnapshot(ctx, bound)
			if err != nil {
				errs <- err
				return
			}
			results <- payload
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	byRevision := map[snapshot.Revision]string{}
	bound := limits
	bound.MaxRows = 20
	for payload := range results {
		p, err := snapshot.Decode(payload, bound)
		if err != nil {
			t.Fatal(err)
		}
		if previous, ok := byRevision[p.Revision]; ok && previous != string(payload) {
			t.Fatalf("revision %d published conflicting bytes", p.Revision)
		}
		byRevision[p.Revision] = string(payload)
	}
	final, err := store.PublishSnapshot(ctx, bound)
	if err != nil {
		t.Fatal(err)
	}
	var p snapshot.Payload
	if err := json.Unmarshal(final, &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 12 || p.Revision < 2 {
		t.Fatalf("final policy: revision %d with %d rows", p.Revision, len(p.Rows))
	}
	for revision := range byRevision {
		if revision > p.Revision {
			t.Fatal("final revision regressed")
		}
	}
	if _, err := store.PublishSnapshot(ctx, limits); err == nil {
		t.Fatal("row bound ignored")
	}
}
