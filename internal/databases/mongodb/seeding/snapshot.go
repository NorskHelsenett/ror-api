package mongodbseeding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/helpers/stringhelper"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclprincipal"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels/aclscope"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const snapshotSeedOwner = "development-acl-snapshot"
const snapshotServiceID = "acl-snapshot"

func seedSnapshotGrant(ctx context.Context, collection *mongo.Collection) error {
	if !rorconfig.GetBool(rorconfig.DEVELOPMENT) || rorconfig.GetString("DEV_ACL_SNAPSHOT_API_KEY") == "" {
		return nil
	}
	grant := aclmodels.AclV3ListItem{
		Version: 3, Group: aclprincipal.Service(snapshotServiceID),
		Scope: aclscope.ScopeRor, Subject: aclscope.SubjectAcl,
		Access:  []aclmodels.AccessTypeV3{aclmodels.CapRorAclSnapshot.WithVerb(aclmodels.VerbRead)},
		Created: time.Now(), IssuedBy: "system@ror.dev",
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	_, err := collection.UpdateOne(ctx, bson.M{
		"version": grant.Version, "group": grant.Group, "scope": grant.Scope,
		"subject": grant.Subject, "access": grant.Access,
	}, bson.M{"$setOnInsert": grant}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("seed development snapshot ACL grant: %w", err)
	}
	return nil
}

func seedSnapshotAPIKey(ctx context.Context, collection *mongo.Collection) error {
	if !rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		return nil
	}
	rorconfig.SetDefault("DEV_ACL_SNAPSHOT_API_KEY", "")
	credential := rorconfig.GetString("DEV_ACL_SNAPSHOT_API_KEY")
	identifier := snapshotServiceID
	if credential == "" {
		return nil
	}
	salt := rorconfig.GetString(rorconfig.ROR_API_KEY_SALT)
	if salt == "" {
		return errors.New("development snapshot seed requires the API key salt")
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	filter := bson.M{"identifier": identifier}
	var existing []struct {
		apicontracts.ApiKey `bson:",inline"`
		SeedOwner           string `bson:"seedOwner"`
	}
	cursor, err := collection.Find(ctx, filter, options.Find().SetLimit(2))
	if err != nil {
		return fmt.Errorf("find development snapshot credential: %w", err)
	}
	if err := cursor.All(ctx, &existing); err != nil {
		return fmt.Errorf("decode development snapshot credentials: %w", err)
	}
	if len(existing) > 1 || (len(existing) == 1 &&
		(existing[0].Type != apicontracts.ApiKeyTypeService || existing[0].SeedOwner != snapshotSeedOwner)) {
		return errors.New("development snapshot identifier conflicts with existing credentials")
	}
	if len(existing) == 1 {
		filter["_id"] = existing[0].Id
		filter["type"] = apicontracts.ApiKeyTypeService
		filter["seedOwner"] = snapshotSeedOwner
	} else {
		// A stable ID makes concurrent initializers target the same owned record.
		filter["_id"] = snapshotSeedOwner + ":" + identifier
		filter["type"] = apicontracts.ApiKeyTypeService
		filter["seedOwner"] = snapshotSeedOwner
	}
	result, err := collection.UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"type":        apicontracts.ApiKeyTypeService,
			"displayname": "ACL snapshot",
			"readonly":    true,
			"expires":     time.Time{},
			"hash":        stringhelper.HashSHA512(credential, []byte(salt)),
		},
		"$setOnInsert": bson.M{"created": time.Now(), "lastused": time.Time{}},
	}, options.UpdateOne().SetUpsert(len(existing) == 0))
	if err != nil {
		return fmt.Errorf("seed development snapshot credential: %w", err)
	}
	if result.MatchedCount == 0 && result.UpsertedCount == 0 {
		return errors.New("development snapshot credential ownership changed during seeding")
	}
	return nil
}
