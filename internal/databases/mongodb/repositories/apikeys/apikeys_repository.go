// TODO: This library is imported from ror, should determine if its a public library or not
package apikey

import (
	"context"
	"fmt"
	"time"

	mongoHelper "github.com/NorskHelsenett/ror-api/internal/helpers/mongoHelper"

	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	collectionName = "apikeys"
)

func GetByHash(ctx context.Context, hashedapikey string) ([]apicontracts.ApiKey, error) {
	var aggregationPipeline = []bson.M{
		{"$match": bson.M{"hash": hashedapikey}},
	}
	var results = make([]apicontracts.ApiKey, 0)
	mongoctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	err := mongodb.Aggregate(mongoctx, collectionName, aggregationPipeline, &results)
	if err != nil {
		return results, fmt.Errorf("error finding apikeys: %v", err)
	}
	return results, nil
}

func GetByIdentifier(ctx context.Context, identifier string) ([]apicontracts.ApiKey, error) {
	var aggregationPipeline = []bson.M{
		{"$match": bson.M{"identifier": identifier}},
	}
	var results = make([]apicontracts.ApiKey, 0)
	mongoctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	err := mongodb.Aggregate(mongoctx, collectionName, aggregationPipeline, &results)
	if err != nil {
		return results, fmt.Errorf("error finding apikeys: %v", err)
	}
	return results, nil
}
func GetByIdentifierAndName(ctx context.Context, identifier string, name string) (*apicontracts.ApiKey, error) {
	var aggregationPipeline = []bson.M{
		{"$match": bson.M{"displayname": name, "identifier": identifier}},
	}
	var results = make([]apicontracts.ApiKey, 0)
	mongoctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	err := mongodb.Aggregate(mongoctx, collectionName, aggregationPipeline, &results)
	if err != nil {
		return nil, fmt.Errorf("error finding apikeys: %v", err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("expected to find one apikey for identifier %s and name %s, found %d", identifier, name, len(results))
	}

	return &results[0], nil
}

func UpdateByNameAndIdentifier(ctx context.Context, identifier string, name string, hash string, expires time.Time) error {
	existing, err := GetByIdentifierAndName(ctx, identifier, name)
	if err != nil {
		return fmt.Errorf("failed to query existing apikey: %v", err)
	}

	objectId, err := primitive.ObjectIDFromHex(existing.Id)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": objectId, "identifier": identifier}
	update := bson.M{"$set": bson.M{"hash": hash, "expires": expires}}

	_, err = mongodb.UpdateOne(ctx, collectionName, filter, update)

	if err != nil {
		return err
	}
	return nil
}

func GetOwnByName(ctx context.Context, name string) (*apicontracts.ApiKey, error) {
	identity := rorcontext.GetIdentityFromRorContext(ctx)

	return GetByIdentifierAndName(ctx, identity.GetId(), name)
}

func UpdateOwnByName(ctx context.Context, name string, hash string, expires time.Time) error {
	identity := rorcontext.GetIdentityFromRorContext(ctx)

	return UpdateByNameAndIdentifier(ctx, identity.GetId(), name, hash, expires)
}

func GetByFilter(ctx context.Context, filter *apicontracts.Filter) ([]apicontracts.ApiKey, int, error) {
	aggregationPipeline := mongoHelper.CreateAggregationPipeline(filter, apicontracts.SortMetadata{SortField: "created", SortOrder: 1}, []string{})

	var results = make([]apicontracts.ApiKey, 0)
	err := mongodb.Aggregate(ctx, collectionName, aggregationPipeline, &results)
	if err != nil {
		return nil, 0, fmt.Errorf("error finding apikeys: %v", err)
	}

	totalCount, err := mongodb.CountWithQuery(ctx, collectionName, aggregationPipeline)
	if err != nil {
		return nil, 0, fmt.Errorf("could not get total count for apikey: %v", err)
	}
	return results, totalCount, nil
}

func Delete(ctx context.Context, ID string) (bool, apicontracts.ApiKey, error) {
	var originalObject apicontracts.ApiKey
	mongoID, err := primitive.ObjectIDFromHex(ID)
	if err != nil {
		return false, originalObject, fmt.Errorf("could not convert ID: %v", err)
	}

	query := bson.M{"_id": mongoID}
	err = mongodb.FindOne(ctx, collectionName, query, &originalObject)
	if err != nil {
		rlog.Error("could not get original object for auditlog", err)
	}

	deleteQuery := bson.M{"_id": mongoID}
	deleteResult, err := mongodb.DeleteOne(ctx, collectionName, deleteQuery)
	if err != nil {
		return false, originalObject, fmt.Errorf("could not delete object: %v", err)
	}

	if deleteResult.DeletedCount == 0 {
		return false, originalObject, fmt.Errorf("could not delete object")
	}

	return true, originalObject, nil
}

func Create(ctx context.Context, input apicontracts.ApiKey) error {
	input.Created = time.Now()
	input.Id = ""

	_, err := mongodb.InsertOne(ctx, collectionName, input)
	if err != nil {
		return fmt.Errorf("could not insert project: %v", err)
	}

	return nil
}

func UpdateLastUsed(ctx context.Context, apikeyId string, identifier string) error {
	mongoID, err := primitive.ObjectIDFromHex(apikeyId)
	if err != nil {
		return fmt.Errorf("could not convert ID: %v", err)
	}

	filter := bson.M{"_id": mongoID, "identifier": identifier}
	update := bson.M{"$set": bson.M{"lastUsed": time.Now()}}

	updateResult, err := mongodb.UpdateOne(ctx, collectionName, filter, update)

	if err != nil {
		return err
	}

	if updateResult.MatchedCount == 0 {
		return fmt.Errorf("could not update object")
	}

	return nil
}
