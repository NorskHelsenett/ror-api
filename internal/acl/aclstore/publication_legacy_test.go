package aclstore

import (
	"context"
	"slices"
	"testing"
	"time"

	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestPublishSnapshotLegacyAndInertMongo(t *testing.T) {
	db, store := newPublicationTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := db.Collection("acl").InsertMany(ctx, []any{
		bson.M{"version": 2, "group": "legacy", "scope": "KubernetesCluster", "subject": "cluster-uid",
			"access": bson.M{"read": true}, "kubernetes": bson.M{"logon": true}},
		bson.M{"version": 2, "group": "inert", "scope": "ror", "subject": "acl",
			"access": bson.M{}, "kubernetes": bson.M{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.PublishSnapshot(ctx, publicationLimits())
	if err != nil {
		t.Fatal(err)
	}
	p, err := snapshot.Decode(data, publicationLimits())
	if err != nil || len(p.Rows) != 2 {
		t.Fatalf("legacy policy: %#v %v", p, err)
	}
	if p.Rows[0].Group != "inert" || p.Rows[0].Grant.Access == nil || len(p.Rows[0].Grant.Access) != 0 {
		t.Fatal("inert legacy grant not preserved")
	}
	if p.Rows[1].Group != "legacy" ||
		!slices.Contains(p.Rows[1].Grant.Access, "ror:read") ||
		!slices.Contains(p.Rows[1].Grant.Access, "kubernetes:logon") {
		t.Fatal("legacy capabilities were lost")
	}
	limits := publicationLimits()
	limits.MaxPayloadBytes = 32
	if _, err := store.PublishSnapshot(ctx, limits); err == nil {
		t.Fatal("oversized authoritative rows accepted")
	}
	_, err = db.Collection("acl").InsertOne(ctx, bson.M{
		"version": 3, "group": "bad", "scope": "ror", "subject": "acl", "access": "not-an-array",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.PublishSnapshot(ctx, publicationLimits()); err == nil {
		t.Fatal("malformed authoritative row accepted")
	}
}
