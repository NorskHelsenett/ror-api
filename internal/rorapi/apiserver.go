package rorapi

import (
	"context"
	"os"
	"os/signal"
	"sync"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/utils/switchboard"
	"github.com/NorskHelsenett/ror-api/internal/webserver"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	healthserver "github.com/NorskHelsenett/ror/pkg/helpers/rorhealth/server"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper/vaulttokenadapter"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/trace"
)

func Run() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var wg sync.WaitGroup

	InitConfig()
	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("API-version: %s (%s) Library-version: %s", rorversion.GetRorVersion().GetVersion(), rorversion.GetRorVersion().GetCommit(), rorversion.GetRorVersion().GetLibVer())

	//TODO: Refactor the init funcitons called to respect context cancelations
	apiconnections.InitConnections(ctx)

	//TODO: refactor the trace package to respect context cancelations
	if rorconfig.GetBool(rorconfig.ENABLE_TRACING) {
		go func() {
			trace.ConnectTracer(done, rorconfig.GetString(rorconfig.TRACER_ID), rorconfig.GetString(rorconfig.OPENTELEMETRY_COLLECTOR_ENDPOINT))
			<-sigs
			done <- struct{}{}
		}()
	}

	webserver.StartListening(ctx, &wg)

	//TODO: refactor health server to respect context cancelations
	healthserver.MustStart(healthserver.ServerString(getHealthEndpoint()))

	if apiconnections.RabbitMQConnection.Ping() {
		// Use of unimplemented code
		switchboard.PublishStarted(ctx)
	}

	tokenstoragehelper.Init(vaulttokenadapter.NewVaultStorageAdapter(apiconnections.VaultClient, rorconfig.GetString("TOKEN_STORE_VAULT_PATH")))

	wg.Wait()

	rlog.Infoc(ctx, "Ror-API shutting down")
}
