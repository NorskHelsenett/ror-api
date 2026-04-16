package apiserver

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/utils/switchboard"
	"github.com/NorskHelsenett/ror-api/internal/webserver"
	"github.com/NorskHelsenett/ror-api/pkg/services/tokenservice"
	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	"github.com/NorskHelsenett/ror/pkg/helpers/oidchelper"
	healthserver "github.com/NorskHelsenett/ror/pkg/helpers/rorhealth/server"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper"
	"github.com/NorskHelsenett/ror/pkg/helpers/tokenstoragehelper/vaulttokenadapter"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// resourcesProcessed is a Prometheus counter for the number of processed resources
	resourcesProcessed = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "ror_api_not_safe_for_production",
		Help: "Bool representing if the ROR-API is running in development mode",
	})
)

func Run() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var wg sync.WaitGroup

	InitConfig()
	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("API-version: %s (%s) Library-version: %s", rorversion.GetRorVersion().GetVersion(), rorversion.GetRorVersion().GetCommit(), rorversion.GetRorVersion().GetLibVer())

	//TODO: Refactor the init functions called to respect context cancelations
	apiconnections.InitConnections(ctx)

	rortracer.InitWithDefault(ctx, rortracer.WithTimeout(time.Second*5))
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rortracer.Shutdown(shutdownCtx)
	}()
	webserver.StartListening(ctx, &wg)

	//TODO: refactor health server to respect context cancelations
	healthserver.MustStart(healthserver.ServerString(getHealthEndpoint()))

	if apiconnections.RabbitMQConnection.Ping() {
		// Use of unimplemented code
		switchboard.PublishStarted(ctx)
	}

	// Initialize token storage for ror-auth
	tokenstoragehelper.Init(vaulttokenadapter.NewVaultStorageAdapter(apiconnections.VaultClient, rorconfig.GetString("TOKEN_STORE_VAULT_PATH")))

	// Initialize token service with OIDC manager (validator + signer)
	oidcConfigs, err := oidchelper.LoadFromEnv()
	if err != nil {
		rlog.Fatal("could not load OIDC configuration", err)
	}
	signerIssuer := rorconfig.GetString(rorconfig.OIDC_SIGNER_ISSUER)
	manager, err := oidchelper.NewManagerWithStorage(signerIssuer, tokenstoragehelper.GetSigningTokenKeyStorage(), oidcConfigs...)
	if err != nil {
		rlog.Fatal("could not initialize OIDC manager", err)
	}
	tokenservice.SetManager(manager)

	// if in development mode, print warning and development api keys
	if rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		printDevelopmentWarning()
		printDevelopemntApiKeys()
		resourcesProcessed.Set(1)
	}

	// Wait for shutdown
	wg.Wait()
	rlog.Infoc(ctx, "Ror-API shutting down")
}
