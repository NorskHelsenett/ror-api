package apiconnections

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice/v2"
	mongodbseeding "github.com/NorskHelsenett/ror-api/internal/databases/mongodb/seeding"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqdefinitions"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqhandler"
	"github.com/NorskHelsenett/ror-api/internal/webserver/sse"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/auth/userauth"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/clients/rabbitmqclient"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/databasecredhelper"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/rabbitmqcredhelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorhealth"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	VaultClient        *vaultclient.VaultClient
	RabbitMQConnection rabbitmqclient.RabbitMQConnection
	DomainResolvers    *userauth.DomainResolvers

	clusterIdToUidCache sync.Map // map[string]string: clusterID -> uid
)

func InitConnections(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	VaultClient = vaultclient.NewVaultClient(rorconfig.GetString(rorconfig.ROLE), rorconfig.GetString(rorconfig.VAULT_URL))

	mongocredshelper := databasecredhelper.NewVaultDBCredentials(VaultClient, rorconfig.GetString(rorconfig.ROLE), "mongodb")
	mongodb.Init(mongocredshelper, rorconfig.GetString(rorconfig.MONGODB_HOST), rorconfig.GetString(rorconfig.MONGODB_PORT), rorconfig.GetString(rorconfig.MONGO_DATABASE))

	aclservice.InitResolver()

	aclmodels.ClusterIdToUidResolver = func(clusterID string) string {
		if uid, ok := clusterIdToUidCache.Load(clusterID); ok {
			return uid.(string)
		}
		db := mongodb.GetMongoDb()
		if db == nil {
			return ""
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Authoritative source: the uid stored on the cluster's apikey. This is
		// deterministic and set at registration (or lazily backfilled), so it is
		// preferred over scanning resourcesv2.
		var apikeyResult struct {
			UID string `bson:"uid"`
		}
		err := db.Collection("apikeys").FindOne(ctx, bson.M{
			"identifier": clusterID,
			"type":       string(apicontracts.ApiKeyTypeCluster),
		}).Decode(&apikeyResult)
		if err == nil && apikeyResult.UID != "" {
			clusterIdToUidCache.Store(clusterID, apikeyResult.UID)
			return apikeyResult.UID
		}

		// Fallback for legacy clusters without a stored apikey uid: deterministically
		// pick the canonical KubernetesCluster (uid == ownerref.subject), then the
		// oldest, so resolution is stable even when duplicates exist.
		pipeline := []bson.M{
			{"$match": bson.M{
				"typemeta.kind": "KubernetesCluster",
				"kubernetescluster.status.agentstatus.clusterid": clusterID,
			}},
			{"$addFields": bson.M{
				"_iscanonical": bson.M{
					"$cond": bson.A{
						bson.M{"$eq": bson.A{"$uid", "$rormeta.ownerref.subject"}}, 0, 1,
					},
				},
			}},
			{"$sort": bson.D{
				{Key: "_iscanonical", Value: 1},
				{Key: "metadata.creationtimestamp.time", Value: 1},
				{Key: "_id", Value: 1},
			}},
			{"$limit": 1},
		}

		cursor, err := db.Collection("resourcesv2").Aggregate(ctx, pipeline)
		if err != nil {
			return ""
		}
		defer func() { _ = cursor.Close(ctx) }()

		var results []struct {
			UID string `bson:"uid"`
		}
		if err := cursor.All(ctx, &results); err != nil || len(results) == 0 {
			return ""
		}

		uid := results[0].UID
		// Only cache non-empty resolutions so a transient miss is not sticky.
		if uid != "" {
			clusterIdToUidCache.Store(clusterID, uid)
		}
		return uid
	}

	rmqcredhelper := rabbitmqcredhelper.NewVaultRMQCredentials(VaultClient, rorconfig.GetString(rorconfig.ROLE))
	RabbitMQConnection = rabbitmqclient.NewRabbitMQConnection(rmqcredhelper, rorconfig.GetString(rorconfig.RABBITMQ_HOST), rorconfig.GetString(rorconfig.RABBITMQ_PORT), rorconfig.GetString(rorconfig.RABBITMQ_BROADCAST_NAME))

	var err error
	DomainResolvers, err = LoadDomainResolvers()
	if err != nil {
		rlog.Error("Failed to load domain resolvers", err)
	}

	DomainResolvers.RegisterHealthChecks()
	rorhealth.Register(ctx, "vault", VaultClient)
	// rorhealth.Register(ctx, "redis", RedisDB)
	rorhealth.Register(ctx, "rabbitmq", RabbitMQConnection)

	apirabbitmqdefinitions.InitOrDie(RabbitMQConnection)
	mongodbseeding.CheckAndSeed(ctx)

	rorconfig.SetWithProvider(rorconfig.ROR_API_KEY_SALT, VaultClient.GetSecretProvider("secret/data/v1.0/ror/config/common", "apikeySalt"))

	apirabbitmqhandler.StartListening(RabbitMQConnection)

	sse.Init(RabbitMQConnection)

}

func LoadDomainResolvers() (*userauth.DomainResolvers, error) {
	vaultconfig, err := VaultClient.GetSecret("secret/data/v1.0/ror/config/auth")
	if err != nil {
		rorerror := rorerror.NewRorError(500, "error getting domain resolvers config from secret provider", err)
		return nil, rorerror
	}
	data := vaultconfig["data"]
	drconfig, err := json.Marshal(data)
	if err != nil {
		rorerror := rorerror.NewRorError(500, "error marshaling secret value to json", err)
		return nil, rorerror
	}

	return userauth.NewDomainResolversFromJson(drconfig)
}
