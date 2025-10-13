package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/NorskHelsenett/ror-api/internal/utils/switchboard"
	"github.com/NorskHelsenett/ror-api/internal/webserver/sse"
	"github.com/NorskHelsenett/ror-api/pkg/servers/sseserver"

	"github.com/NorskHelsenett/ror-api/internal/apiconfig"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	mongodbseeding "github.com/NorskHelsenett/ror-api/internal/databases/mongodb/seeding"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqdefinitions"
	"github.com/NorskHelsenett/ror-api/internal/rabbitmq/apirabbitmqhandler"
	"github.com/NorskHelsenett/ror-api/internal/utils"
	"github.com/NorskHelsenett/ror-api/internal/webserver"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"

	"github.com/NorskHelsenett/ror/pkg/clients/mongodb"
	"github.com/NorskHelsenett/ror/pkg/clients/vaultclient/databasecredhelper"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/telemetry/trace"

	healthserver "github.com/NorskHelsenett/ror/pkg/helpers/rorhealth/server"

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
	rorconfig.InitConfig()
	apiconfig.InitViper()

	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("API-version: %s (%s) Library-version: %s", rorversion.GetRorVersion().GetVersion(), rorversion.GetRorVersion().GetCommit(), rorversion.GetRorVersion().GetLibVer())

	_, _ = maxprocs.Set(maxprocs.Logger(rlog.Infof))

	apiconnections.InitConnections()

	utils.GetCredsFromVault()

	mongocredshelper := databasecredhelper.NewVaultDBCredentials(apiconnections.VaultClient, rorconfig.GetString(rorconfig.ROLE), "mongodb")
	mongodb.Init(mongocredshelper, rorconfig.GetString(rorconfig.MONGODB_HOST), rorconfig.GetString(rorconfig.MONGODB_PORT), rorconfig.GetString(rorconfig.MONGO_DATABASE))

	apirabbitmqdefinitions.InitOrDie()
	mongodbseeding.CheckAndSeed(ctx)
	sse.Init()

	rorconfig.SetDefault(rorconfig.ROLE, "ror-api")
	rorconfig.SetWithProvider(rorconfig.API_KEY_SALT, apiconnections.VaultClient.GetSecretProvider("secret/data/v1.0/ror/config/common", "apikeySalt"))

	if rorconfig.GetBool(rorconfig.OIDC_SKIP_ISSUER_VERIFY) {
		rlog.Error("skipping OIDC issuer verification. THIS IS UNSAFE IN PRODUCTION!!!", nil)
	}

	if rorconfig.GetBool(rorconfig.ENABLE_TRACING) {

		go func() {
			trace.ConnectTracer(done, rorconfig.GetString(rorconfig.TRACER_ID), rorconfig.GetString(rorconfig.OPENTELEMETRY_COLLECTOR_ENDPOINT))
			<-sigs
			done <- struct{}{}
		}()
	}

	go func() {
		webserver.InitHttpServer()
		<-sigs
		done <- struct{}{}
	}()
	sseserver.StartEventServer()
	rlog.Infoc(ctx, "Initializing health server", rlog.String("endpoint", apiconfig.GetHealthEndpoint()))
	err := healthserver.Start(healthserver.ServerString(apiconfig.GetHealthEndpoint()))

	if err != nil {
		rlog.Error("Failed to start health server", err)
		os.Exit(1)
	}

	if apiconnections.RabbitMQConnection.Ping() {
		switchboard.PublishStarted(ctx)
	}

	apirabbitmqhandler.StartListening()

	<-done
	rlog.Infoc(ctx, "Ror-API shutting down")
}
