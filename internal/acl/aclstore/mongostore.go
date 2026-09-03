package aclstore

import (
	"context"
	"fmt"
	"time"

	aclstorev2 "github.com/NorskHelsenett/ror/pkg/acl/aclstore/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const aclCollectionName = "acl"

// aclVersions is the set of document versions the store reads. V2 documents are
// converted to V3 on read; new writes are always persisted as V3.
var aclVersions = bson.A{2, 3}

// MongoStore is the MongoDB-backed implementation of the ACL Store. It reads the
// mixed V2/V3 documents in the acl collection and returns them as canonical V3,
// and persists all writes as V3.
type MongoStore struct {
	// dbProvider returns the live *mongo.Database on every call. It must not be
	// cached: the mongo client is reconnected (and the previous one disconnected)
	// on credential rotation, so a captured handle would start failing with
	// "client is disconnected" after the first renewal.
	dbProvider func() *mongo.Database
}

// compile-time assurance that MongoStore satisfies the shared v2 interface.
var _ aclstorev2.Store = (*MongoStore)(nil)

// NewMongoStore creates a MongoDB-backed ACL store. dbProvider must return the
// current *mongo.Database; it is called on every operation (see the field doc).
func NewMongoStore(dbProvider func() *mongo.Database) *MongoStore {
	return &MongoStore{dbProvider: dbProvider}
}

// aclVersionProbe decodes only the version discriminator of a stored document.
type aclVersionProbe struct {
	Version int `bson:"version"`
}

// decodeCursor decodes the cursor's current document as a canonical V3 item,
// converting from V2 when necessary. It uses cursor.Decode (not raw
// bson.Unmarshal) so the client's BSON options, notably ObjectIDAsHexString,
// are honoured when mapping the ObjectID _id into the model's string id.
func decodeCursor(cursor *mongo.Cursor) (aclmodels.AclV3ListItem, error) {
	var probe aclVersionProbe
	if err := cursor.Decode(&probe); err != nil {
		return aclmodels.AclV3ListItem{}, fmt.Errorf("failed to decode ACL entry version: %w", err)
	}

	switch probe.Version {
	case 3:
		var entry aclmodels.AclV3ListItem
		if err := cursor.Decode(&entry); err != nil {
			return aclmodels.AclV3ListItem{}, fmt.Errorf("failed to decode V3 ACL entry: %w", err)
		}
		return entry, nil
	case 2:
		var entry aclmodels.AclV2ListItem
		if err := cursor.Decode(&entry); err != nil {
			return aclmodels.AclV3ListItem{}, fmt.Errorf("failed to decode V2 ACL entry: %w", err)
		}
		return aclmodels.V2ToV3(entry), nil
	default:
		return aclmodels.AclV3ListItem{}, fmt.Errorf("unknown ACL entry version %d", probe.Version)
	}
}

// find runs a query and returns all matching documents as canonical V3 items.
func (s *MongoStore) find(ctx context.Context, filter bson.M) ([]aclmodels.AclV3ListItem, error) {
	db := s.dbProvider()
	if db == nil {
		return nil, fmt.Errorf("mongodb not initialized")
	}

	cursor, err := db.Collection(aclCollectionName).Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query ACL entries: %w", err)
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			rlog.Error("failed to close ACL cursor", err)
		}
	}()

	var result []aclmodels.AclV3ListItem
	for cursor.Next(ctx) {
		entry, err := decodeCursor(cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error reading ACL entries: %w", err)
	}
	return result, nil
}

// getByObjectID returns the entry with the given _id as a canonical V3 item, or
// nil if it does not exist.
func (s *MongoStore) getByObjectID(ctx context.Context, oid bson.ObjectID) (*aclmodels.AclV3ListItem, error) {
	entries, err := s.find(ctx, bson.M{"_id": oid})
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}
	return &entries[0], nil
}

// GetByGroups returns ACL entries for the given groups as V3 items, keyed by group.
func (s *MongoStore) GetByGroups(ctx context.Context, groups []string) (aclmodels.AclV3List, error) {
	entries, err := s.find(ctx, bson.M{
		"version": bson.M{"$in": aclVersions},
		"group":   bson.M{"$in": groups},
	})
	if err != nil {
		return nil, err
	}

	result := make(aclmodels.AclV3ListByGroup, len(groups))
	for _, entry := range entries {
		result[entry.Group] = append(result[entry.Group], entry)
	}
	return result.Flatten(), nil
}

// GetByScopeSubject returns all ACL entries for the scope+subject pair as V3 items.
func (s *MongoStore) GetByScopeSubject(ctx context.Context, scope aclscope.Scope, subject aclscope.Subject) (aclmodels.AclV3List, error) {
	return s.find(ctx, bson.M{
		"version": bson.M{"$in": aclVersions},
		"scope":   scope,
		"subject": subject,
	})
}

// GetById returns a single ACL entry by id as a V3 item, or nil if not found.
func (s *MongoStore) GetById(ctx context.Context, id string) (*aclmodels.AclV3ListItem, error) {
	db := s.dbProvider()
	if db == nil {
		return nil, fmt.Errorf("mongodb not initialized")
	}

	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id %q: %w", id, err)
	}

	return s.getByObjectID(ctx, oid)
}

// GetAll returns every ACL entry as V3 items. Bulk-load primitive; not for hot paths.
func (s *MongoStore) GetAll(ctx context.Context) (aclmodels.AclV3List, error) {
	return s.find(ctx, bson.M{"version": bson.M{"$in": aclVersions}})
}

// Create persists a new ACL entry as V3 and returns the stored item.
func (s *MongoStore) Create(ctx context.Context, item aclmodels.AclV3ListItem) (*aclmodels.AclV3ListItem, error) {
	db := s.dbProvider()
	if db == nil {
		return nil, fmt.Errorf("mongodb not initialized")
	}

	item.Version = 3
	item.Id = "" // let mongo assign _id
	if item.Created.IsZero() {
		item.Created = time.Now()
	}

	res, err := db.Collection(aclCollectionName).InsertOne(ctx, item)
	if err != nil {
		return nil, fmt.Errorf("could not insert ACL entry: %w", err)
	}

	oid, ok := res.InsertedID.(bson.ObjectID)
	if !ok {
		return nil, fmt.Errorf("unexpected inserted id type %T", res.InsertedID)
	}
	stored, err := s.getByObjectID(ctx, oid)
	if err != nil {
		return nil, fmt.Errorf("could not read back inserted ACL entry: %w", err)
	}
	if stored == nil {
		return nil, fmt.Errorf("inserted ACL entry not found on read back")
	}
	return stored, nil
}

// Update replaces the entry with the given id, returning the updated and previous items.
func (s *MongoStore) Update(ctx context.Context, id string, item aclmodels.AclV3ListItem) (*aclmodels.AclV3ListItem, *aclmodels.AclV3ListItem, error) {
	db := s.dbProvider()
	if db == nil {
		return nil, nil, fmt.Errorf("mongodb not initialized")
	}

	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid id %q: %w", id, err)
	}

	previous, err := s.getByObjectID(ctx, oid)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read existing ACL entry: %w", err)
	}
	if previous == nil {
		return nil, nil, fmt.Errorf("ACL entry %q not found", id)
	}

	item.Version = 3
	// _id is immutable in mongo; clearing it avoids a "changed _id" error on replace.
	item.Id = ""
	item.Created = previous.Created
	item.IssuedBy = previous.IssuedBy

	upd, err := db.Collection(aclCollectionName).ReplaceOne(ctx, bson.M{"_id": oid}, item)
	if err != nil {
		return nil, nil, fmt.Errorf("could not update ACL entry: %w", err)
	}
	if upd.MatchedCount == 0 {
		return nil, nil, fmt.Errorf("ACL entry %q not found", id)
	}

	updated, err := s.getByObjectID(ctx, oid)
	if err != nil {
		return nil, nil, fmt.Errorf("could not read back updated ACL entry: %w", err)
	}
	if updated == nil {
		return nil, nil, fmt.Errorf("updated ACL entry not found on read back")
	}
	return updated, previous, nil
}

// Delete removes the entry with the given id and returns the deleted item.
func (s *MongoStore) Delete(ctx context.Context, id string) (*aclmodels.AclV3ListItem, error) {
	db := s.dbProvider()
	if db == nil {
		return nil, fmt.Errorf("mongodb not initialized")
	}

	oid, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id %q: %w", id, err)
	}

	deleted, err := s.getByObjectID(ctx, oid)
	if err != nil {
		return nil, fmt.Errorf("failed to read ACL entry before delete: %w", err)
	}
	if deleted == nil {
		return nil, fmt.Errorf("ACL entry %q not found", id)
	}

	del, err := db.Collection(aclCollectionName).DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return nil, fmt.Errorf("could not delete ACL entry: %w", err)
	}
	if del.DeletedCount == 0 {
		return nil, fmt.Errorf("ACL entry %q not found", id)
	}
	return deleted, nil
}
