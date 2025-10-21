package apiconnections

import (
	"context"
	"encoding/json"
	"fmt"

	mongodbseeding "github.com/NorskHelsenett/ror-api/internal/databases/mongodb/seeding"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqdefinitions"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqhandler"
	"github.com/NorskHelsenett/ror-api/internal/webserver/sse"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/auth/userauth"
	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/clients/rabbitmqclient"

	"github.com/NorskHelsenett/ror/pkg/clients/redisdb"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/databasecredhelper"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/rabbitmqcredhelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorhealth"
)

var (
	VaultClient        *vaultclient.VaultClient
	RedisDB            redisdb.RedisDB
	RabbitMQConnection rabbitmqclient.RabbitMQConnection
	DomainResolvers    *userauth.DomainResolvers
)

func InitConnections() {
	VaultClient = vaultclient.NewVaultClient(rorconfig.GetString(rorconfig.ROLE), rorconfig.GetString(rorconfig.VAULT_URL))

	mongocredshelper := databasecredhelper.NewVaultDBCredentials(VaultClient, rorconfig.GetString(rorconfig.ROLE), "mongodb")
	mongodb.Init(mongocredshelper, rorconfig.GetString(rorconfig.MONGODB_HOST), rorconfig.GetString(rorconfig.MONGODB_PORT), rorconfig.GetString(rorconfig.MONGO_DATABASE))

	redisdatabasecredhelper := databasecredhelper.NewVaultDBCredentials(VaultClient, fmt.Sprintf("redis-%v-role", rorconfig.GetString(rorconfig.ROLE)), "")
	RedisDB = redisdb.New(redisdatabasecredhelper, rorconfig.GetString(rorconfig.KV_HOST), rorconfig.GetString(rorconfig.KV_PORT))

	rmqcredhelper := rabbitmqcredhelper.NewVaultRMQCredentials(VaultClient, rorconfig.GetString(rorconfig.ROLE))
	RabbitMQConnection = rabbitmqclient.NewRabbitMQConnection(rmqcredhelper, rorconfig.GetString(rorconfig.RABBITMQ_HOST), rorconfig.GetString(rorconfig.RABBITMQ_PORT), rorconfig.GetString(rorconfig.RABBITMQ_BROADCAST_NAME))

	var err error
	DomainResolvers, err = LoadDomainResolvers()
	if err != nil {
		rlog.Error("Failed to load domain resolvers", err)
	}

	DomainResolvers.RegisterHealthChecks()
	rorhealth.Register("vault", VaultClient)
	rorhealth.Register("redis", RedisDB)
	rorhealth.Register("rabbitmq", RabbitMQConnection)

	apirabbitmqdefinitions.InitOrDie(RabbitMQConnection)
	mongodbseeding.CheckAndSeed(context.Background())

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
