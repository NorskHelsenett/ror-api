package aclstore

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclgrants"
	snapshot "github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclsnapshot"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const publicationCollection = "acl_snapshot_publication"
const publicationID = "authoritative"

type publication struct {
	Revision    string        `bson:"revision"`
	ContentHash string        `bson:"contentHash"`
	Payload     []byte        `bson:"payload"`
	Observation bson.ObjectID `bson:"observation,omitempty"`
}

// PublishSnapshot reads authoritative ACLs, never the process-local resolver
// cache. A serialized durable generation is assigned to each observed content
// change. Intermediate policies between publications need not get a generation.
// Single-document compare-and-swap serializes publications on standalone MongoDB
// too. The ACL scan itself is not a point-in-time view during concurrent edits.
func (s *MongoStore) PublishSnapshot(ctx context.Context, limits snapshot.Limits) ([]byte, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	db := s.dbProvider()
	if db == nil {
		return nil, fmt.Errorf("mongodb not initialized")
	}
	collection := db.Collection(publicationCollection)
	filter := bson.M{"_id": publicationID}
	_, err := collection.UpdateOne(ctx, filter,
		bson.M{"$setOnInsert": bson.M{"revision": "0"}}, options.UpdateOne().SetUpsert(true))
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return nil, fmt.Errorf("initialize ACL publication: %w", err)
	}
	for range 32 {
		var current publication
		if err := collection.FindOne(ctx, filter).Decode(&current); err != nil {
			return nil, fmt.Errorf("read ACL publication: %w", err)
		}
		rows, err := snapshotRows(ctx, db, limits)
		if err != nil {
			return nil, err
		}
		next, err := nextPublication(current, rows, limits, time.Now().UTC().Truncate(time.Second))
		if err != nil {
			return nil, err
		}
		// Replace the observation token even for unchanged content. If another
		// publisher won since our read, discard these rows and scan again.
		condition := bson.M{"_id": publicationID, "observation": current.Observation}
		if current.Observation.IsZero() {
			condition["observation"] = bson.M{"$exists": false}
		}
		next.Observation = bson.NewObjectID()
		result, err := collection.UpdateOne(ctx, condition, bson.M{"$set": next})
		if err != nil {
			return nil, fmt.Errorf("persist ACL publication: %w", err)
		}
		if result.MatchedCount == 1 {
			return next.Payload, nil
		}
	}
	return nil, errors.New("ACL publication contention exceeded 32 attempts")
}

func snapshotRows(ctx context.Context, db *mongo.Database, limits snapshot.Limits) ([]snapshot.Row, error) {
	cursor, err := db.Collection(aclCollectionName).Find(ctx,
		bson.M{"version": bson.M{"$in": aclVersions}},
		options.Find().SetLimit(int64(limits.MaxRows)+1).
			SetSort(bson.D{{Key: "_id", Value: 1}}).
			SetProjection(bson.M{"_id": 0, "version": 1, "group": 1, "scope": 1, "subject": 1, "access": 1, "kubernetes": 1}))
	if err != nil {
		return nil, fmt.Errorf("read authoritative ACL rows: %w", err)
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			rlog.Errorc(ctx, "close ACL snapshot cursor", err)
		}
	}()
	rows := []snapshot.Row{}
	size := 2 // JSON array delimiters; the final encoder also bounds metadata.
	for cursor.Next(ctx) {
		if len(rows) == limits.MaxRows {
			return nil, fmt.Errorf("authoritative ACL policy exceeds row bound")
		}
		entry, err := decodeCursor(cursor)
		if err != nil {
			return nil, err
		}
		// In particular, legacy V2 conversion represents an inert grant as nil.
		// The wire contract represents that same grant with an explicit array.
		if entry.Access == nil {
			entry.Access = []aclmodels.AccessTypeV3{}
		}
		row := snapshot.Row{Group: entry.Group, Grant: aclgrants.Grant{
			Scope: entry.Scope, Subject: entry.Subject, Access: entry.Access,
		}}
		encoded, err := json.Marshal(row)
		if err != nil {
			return nil, fmt.Errorf("encode authoritative ACL row: %w", err)
		}
		if len(encoded) > limits.MaxPayloadBytes-size {
			return nil, fmt.Errorf("authoritative ACL policy exceeds payload byte bound")
		}
		size += len(encoded) + 1
		rows = append(rows, row)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("read authoritative ACL cursor: %w", err)
	}
	return rows, nil
}

func nextPublication(current publication, rows []snapshot.Row, limits snapshot.Limits, now time.Time) (publication, error) {
	canonical, err := snapshot.CanonicalRows(rows, limits)
	if err != nil {
		return publication{}, err
	}
	content, err := json.Marshal(canonical)
	if err != nil {
		return publication{}, fmt.Errorf("encode ACL content: %w", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(content))
	revision, err := strconv.ParseUint(current.Revision, 10, 64)
	if err != nil || strconv.FormatUint(revision, 10) != current.Revision {
		return publication{}, fmt.Errorf("stored ACL publication revision is corrupt")
	}
	if revision > 0 {
		// Current deployment bounds constrain new output, not historical data.
		// The stored byte length bounds its own rows/fields, allowing a smaller
		// valid policy to replace a formerly larger one after limits tighten.
		storedLimits := limits
		storedLimits.MaxPayloadBytes = len(current.Payload)
		storedLimits.MaxRows = len(current.Payload)
		storedLimits.MaxAccessEntries = len(current.Payload)
		storedLimits.MaxFieldBytes = len(current.Payload)
		persisted, err := snapshot.Decode(current.Payload, storedLimits)
		if err != nil {
			return publication{}, fmt.Errorf("stored ACL publication is corrupt: %w", err)
		}
		storedRows, err := snapshot.CanonicalRows(persisted.Rows, storedLimits)
		if err != nil {
			return publication{}, err
		}
		storedContent, err := json.Marshal(storedRows)
		if err != nil {
			return publication{}, fmt.Errorf("encode stored ACL content: %w", err)
		}
		if uint64(persisted.Revision) != revision || current.ContentHash != fmt.Sprintf("%x", sha256.Sum256(storedContent)) {
			return publication{}, errors.New("stored ACL publication metadata does not match payload")
		}
		if current.ContentHash == hash {
			if len(current.Payload) > limits.MaxPayloadBytes {
				return publication{}, fmt.Errorf("stored ACL publication exceeds configured payload byte bound")
			}
			return current, nil
		}
	} else if len(current.Payload) != 0 || current.ContentHash != "" {
		return publication{}, errors.New("stored initial ACL publication is corrupt")
	}
	if revision == math.MaxUint64 {
		return publication{}, errors.New("ACL publication revision exhausted")
	}
	payload, err := snapshot.Encode(snapshot.Payload{
		Schema: snapshot.Schema, Revision: snapshot.Revision(revision + 1),
		GeneratedAt: now, Rows: canonical,
	}, limits)
	if err != nil {
		return publication{}, err
	}
	return publication{
		Revision: strconv.FormatUint(revision+1, 10), ContentHash: hash, Payload: payload,
	}, nil
}
