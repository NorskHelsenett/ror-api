package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/NorskHelsenett/ror-api/internal/utils/switchboard"
	"github.com/NorskHelsenett/ror-api/internal/webserver/sse"

	"github.com/NorskHelsenett/ror-api/internal/apiconfig"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	mongodbseeding "github.com/NorskHelsenett/ror-api/internal/databases/mongodb/seeding"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqdefinitions"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqhandler"
	"github.com/NorskHelsenett/ror-api/internal/utils"
	"github.com/NorskHelsenett/ror-api/internal/webserver"

	"github.com/NorskHelsenett/ror/pkg/config/configconsts"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/databasecredhelper"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/telemetry/trace"

	healthserver "github.com/NorskHelsenett/ror/pkg/helpers/rorhealth/server"
	"github.com/spf13/viper"

	"go.uber.org/automaxprocs/maxprocs"
)

//	@title			Swagger ROR-API
//	@version		0.1
//	@description	ROR-API, need any help? Go to channel #drift-sdi-devops in norskhelsenett.slack.com slack workspace
//	@BasePath		/

//	@contact.name	ROR
//	@contact.url	https://github.com/NorskHelsenett/ror

//	@securityDefinitions.apikey	AccessToken
//	@in							header
//	@name						Authorization
//	@securityDefinitions.apikey	ApiKey
//	@in							header
//	@name						X-API-KEY

func main() {
	// rebuild: 3
	ctx := context.Background()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})

	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("Version: %s (%s)", apiconfig.Version, apiconfig.Commit)

	_, _ = maxprocs.Set(maxprocs.Logger(rlog.Infof))

	apiconfig.InitViper()

	apiconnections.InitConnections()

	utils.GetCredsFromVault()

	mongocredshelper := databasecredhelper.NewVaultDBCredentials(apiconnections.VaultClient, viper.GetString(configconsts.ROLE), "mongodb")
	mongodb.Init(mongocredshelper, viper.GetString(configconsts.MONGODB_HOST), viper.GetString(configconsts.MONGODB_PORT), viper.GetString(configconsts.MONGO_DATABASE))

	apirabbitmqdefinitions.InitOrDie()
	mongodbseeding.CheckAndSeed(ctx)
	sse.Init()

	if viper.GetBool(configconsts.OIDC_SKIP_ISSUER_VERIFY) {
		rlog.Error("skipping OIDC issuer verification. THIS IS UNSAFE IN PRODUCTION!!!", nil)
	}

	if viper.GetBool(configconsts.ENABLE_TRACING) {

		go func() {
			trace.ConnectTracer(done, viper.GetString(configconsts.TRACER_ID), viper.GetString(configconsts.OPENTELEMETRY_COLLECTOR_ENDPOINT))
			<-sigs
			done <- struct{}{}
		}()
	}

	go func() {
		webserver.InitHttpServer()
		<-sigs
		done <- struct{}{}
	}()

	rlog.Infoc(ctx, "Initializing health server")
	_ = healthserver.Start(healthserver.ServerString(viper.GetString(configconsts.HEALTH_ENDPOINT)))

	if apiconnections.RabbitMQConnection.Ping() {
		switchboard.PublishStarted(ctx)
	}

	apirabbitmqhandler.StartListening()

	<-done
	rlog.Infoc(ctx, "Ror-API shutting down")
}
