package rorapi

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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
	ctx = context.Background()
	sigs = make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	done = make(chan struct{})
	InitConfig()
	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("API-version: %s (%s) Library-version: %s", rorversion.GetRorVersion().GetVersion(), rorversion.GetRorVersion().GetCommit(), rorversion.GetRorVersion().GetLibVer())
	apiconnections.InitConnections()

	if rorconfig.GetBool(rorconfig.ENABLE_TRACING) {
		go func() {
			trace.ConnectTracer(done, rorconfig.GetString(rorconfig.TRACER_ID), rorconfig.GetString(rorconfig.OPENTELEMETRY_COLLECTOR_ENDPOINT))
			<-sigs
			done <- struct{}{}
		}()
	}

	webserver.StartListening(sigs, done)

	healthserver.MustStart(healthserver.ServerString(getHealthEndpoint()))

	if apiconnections.RabbitMQConnection.Ping() {
		switchboard.PublishStarted(ctx)
	}
	tokenstoragehelper.Init(vaulttokenadapter.NewVaultStorageAdapter(apiconnections.VaultClient, rorconfig.GetString("TOKEN_STORE_VAULT_PATH")))
	<-done
	rlog.Infoc(ctx, "Ror-API shutting down")
}
