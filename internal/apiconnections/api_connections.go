package apiconnections

import (
	"encoding/json"
	"fmt"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/auth/userauth"
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

	redisdatabasecredhelper := databasecredhelper.NewVaultDBCredentials(VaultClient, fmt.Sprintf("redis-%v-role", rorconfig.GetString(rorconfig.ROLE)), "")
	RedisDB = redisdb.New(redisdatabasecredhelper, rorconfig.GetString(rorconfig.REDIS_HOST), rorconfig.GetString(rorconfig.REDIS_PORT))

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
