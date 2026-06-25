package apiconnections

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/NorskHelsenett/ror/pkg/clients/redisdb"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/databasecredhelper"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/rabbitmqcredhelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"go.mongodb.org/mongo-driver/v2/bson"
)

var (
	VaultClient        *vaultclient.VaultClient
	RabbitMQConnection rabbitmqclient.RabbitMQConnection
	RedisDB            redisdb.RedisDB
	DomainResolvers    *userauth.DomainResolvers

	clusterIdToUidCache sync.Map // map[string]string: clusterID -> uid
)

// initConnectionsTimeout bounds how long InitConnections waits for the backing
// dependencies (vault, mongodb, rabbitmq, redis) to become available before
// giving up.
var initConnectionsTimeout = 300 * time.Second

func InitConnections(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, initConnectionsTimeout)
	defer cancel()
	// Vault
	VaultClient = vaultclient.MustNewVaultClientWithContext(ctx, rorconfig.GetString(rorconfig.ROLE), rorconfig.GetString(rorconfig.VAULT_URL))

	//MongoDB
	mongocredshelper := databasecredhelper.NewVaultDBCredentials(VaultClient, rorconfig.GetString(rorconfig.ROLE), "mongodb")
	mongodb.MustInitWithContext(ctx, mongocredshelper, rorconfig.GetString(rorconfig.MONGODB_HOST), rorconfig.GetString(rorconfig.MONGODB_PORT), rorconfig.GetString(rorconfig.MONGO_DATABASE))

	mongodbseeding.CheckAndSeed(ctx)

	// RabbitMQ
	rmqcredhelper := rabbitmqcredhelper.NewVaultRMQCredentials(VaultClient, rorconfig.GetString(rorconfig.ROLE))
	RabbitMQConnection = rabbitmqclient.MustNewRabbitMQConnectionWithContext(ctx, rmqcredhelper, rorconfig.GetString(rorconfig.RABBITMQ_HOST), rorconfig.GetString(rorconfig.RABBITMQ_PORT), rorconfig.GetString(rorconfig.RABBITMQ_BROADCAST_NAME))

	apirabbitmqdefinitions.InitOrDie(RabbitMQConnection)
	apirabbitmqhandler.StartListening(RabbitMQConnection)

	// Redis (valkey)
	rediscredhelper := databasecredhelper.NewVaultDBCredentials(VaultClient, getRedisVaultRole(), "")
	RedisDB = redisdb.MustNewWithContext(ctx, rediscredhelper, rorconfig.GetString(rorconfig.KV_HOST), rorconfig.GetString(rorconfig.KV_PORT))

	// ACL service resolver setup
	aclservice.InitResolver(RedisDB)
	aclmodels.ClusterIdToUidResolver = resolveClusterIdToUid

	// Domain resolvers are initialized up front as an empty, ready-to-use
	// registry so consumers never see a nil resolver, then populated
	// asynchronously. Building each resolver connects to an external directory
	// (LDAP/AD/msgraph) which can be slow or unavailable, so it must not block
	// startup. Additional resolvers can be added later via the same primitive.
	DomainResolvers = userauth.NewDomainResolvers()
	go loadDomainResolversAsync()

	// vault health is registered by MustNewVaultClientWithContext during connect
	// mongodb health is registered by MustInitWithContext during connect
	// rabbitmq health is registered by MustNewRabbitMQConnectionWithContext during connect
	// redis health is registered by MustNewWithContext during connect
	// domainresolver health is registered per resolver as they connect

	rorconfig.SetWithProvider(rorconfig.ROR_API_KEY_SALT, VaultClient.GetSecretProvider("secret/data/v1.0/ror/config/common", "apikeySalt"))

	sse.Init(RabbitMQConnection)

}

// resolveClusterIdToUid resolves a cluster ID (human-readable name) to its UID
// (UUID) using the database. It is injected into aclmodels.ClusterIdToUidResolver
// so the db-agnostic acl model layer can perform the lookup without depending on
// the mongodb client. Results are cached in clusterIdToUidCache; only non-empty
// resolutions are cached so a transient miss is not sticky.
func resolveClusterIdToUid(clusterID string) string {
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
		// Only carry the fields needed for canonical selection, sorting and
		// the returned uid so the full cluster document is not pulled through
		// $sort or over the wire.
		{"$project": bson.M{
			"uid":                             1,
			"rormeta.ownerref.subject":        1,
			"metadata.creationtimestamp.time": 1,
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

// loadDomainResolversAsync fetches the domain resolver configuration from vault
// and builds each resolver in its own goroutine, adding it to the shared registry
// as it connects. Running per-resolver means a slow or unreachable directory
// server delays only its own domain, not startup or the other resolvers.
func loadDomainResolversAsync() {
	drconfig, err := getDomainResolverConfig()
	if err != nil {
		rlog.Error("failed to load domain resolver config", err)
		return
	}

	configs, err := userauth.ParseDomainResolverConfigs(drconfig)
	if err != nil {
		rlog.Error("failed to parse domain resolver config", err)
		return
	}

	for _, cfg := range configs {
		go func(cfg userauth.DomainResolverConfig) {
			if err := DomainResolvers.AddResolverFromConfig(cfg); err != nil {
				rlog.Error("failed to load domain resolver", err, rlog.String("resolverType", cfg.ResolverType))
				return
			}
			rlog.Info("domain resolver loaded", rlog.String("resolverType", cfg.ResolverType))
		}(cfg)
	}
}

// getRedisVaultRole returns the vault database role used to issue redis/valkey
// credentials. It uses KV_VAULT_ROLE when set, otherwise defaults to the
// convention "valkey-<ROLE>-role".
func getRedisVaultRole() string {
	if role := rorconfig.GetString(rorconfig.KV_VAULT_ROLE); role != "" {
		return role
	}
	return fmt.Sprintf("valkey-%s-role", rorconfig.GetString(rorconfig.ROLE))
}

// getDomainResolverConfig reads the raw domain resolver configuration json from
// vault.
func getDomainResolverConfig() ([]byte, error) {
	vaultconfig, err := VaultClient.GetSecret("secret/data/v1.0/ror/config/auth")
	if err != nil {
		return nil, rorerror.NewRorError(500, "error getting domain resolvers config from secret provider", err)
	}
	drconfig, err := json.Marshal(vaultconfig["data"])
	if err != nil {
		return nil, rorerror.NewRorError(500, "error marshaling secret value to json", err)
	}
	return drconfig, nil
}
