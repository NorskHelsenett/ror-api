package mongodbseeding

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/helpers/stringhelper"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func seedSnapshotAPIKey(ctx context.Context, collection *mongo.Collection) error {
	if !rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		return nil
	}
	rorconfig.SetDefault("DEV_ACL_SNAPSHOT_API_KEY", "")
	rorconfig.SetDefault("ACL_SNAPSHOT_READER_SERVICE_ID", "")
	credential := rorconfig.GetString("DEV_ACL_SNAPSHOT_API_KEY")
	identifier := rorconfig.GetString("ACL_SNAPSHOT_READER_SERVICE_ID")
	if credential == "" && identifier == "" {
		return nil
	}
	if credential == "" || identifier == "" {
		return errors.New("development snapshot seed requires DEV_ACL_SNAPSHOT_API_KEY and ACL_SNAPSHOT_READER_SERVICE_ID; wait for local Compose bootstrap")
	}
	salt := rorconfig.GetString(rorconfig.ROR_API_KEY_SALT)
	if salt == "" {
		return errors.New("development snapshot seed requires the API key salt")
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	filter := bson.M{"identifier": identifier}
	var existing []apicontracts.ApiKey
	cursor, err := collection.Find(ctx, filter, options.Find().SetLimit(2))
	if err != nil {
		return fmt.Errorf("find development snapshot credential: %w", err)
	}
	if err := cursor.All(ctx, &existing); err != nil {
		return fmt.Errorf("decode development snapshot credentials: %w", err)
	}
	if len(existing) > 1 || (len(existing) == 1 && existing[0].Type != apicontracts.ApiKeyTypeService) {
		return errors.New("development snapshot identifier conflicts with existing credentials")
	}
	_, err = collection.UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"type":        apicontracts.ApiKeyTypeService,
			"displayname": "ACL snapshot",
			"readonly":    true,
			"expires":     time.Time{},
			"hash":        stringhelper.HashSHA512(credential, []byte(salt)),
		},
		"$setOnInsert": bson.M{"created": time.Now(), "lastused": time.Time{}},
	}, options.UpdateOne().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("seed development snapshot credential: %w", err)
	}
	return nil
}
