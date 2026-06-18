package apiserver

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/apikeyauth"
	"github.com/NorskHelsenett/ror-api/internal/utils/switchboard"
	"github.com/NorskHelsenett/ror-api/internal/webserver"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware/oauthmiddleware"
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

// oidcStartupReadyTimeout bounds how long /health/ready is held unready waiting
// for the initial OIDC issuer discovery to complete during startup.
const oidcStartupReadyTimeout = 30 * time.Second

func Run() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	var wg sync.WaitGroup

	InitConfig()
	rlog.Infoc(ctx, "ROR Api startup ")
	rlog.Infof("API-version: %s (%s) Library-version: %s", rorversion.GetRorVersion().GetVersion(), rorversion.GetRorVersion().GetCommit(), rorversion.GetRorVersion().GetLibVer())

	// Start the health server first so the status is queryable while
	// dependencies are still connecting. Dependency health checks register
	// themselves during InitConnections and are reported as they come up.
	//TODO: refactor health server to respect context cancelations
	healthserver.MustStart(healthserver.ServerString(getHealthEndpoint()))

	//TODO: Refactor the init functions called to respect context cancelations
	apiconnections.InitConnections(ctx)

	rortracer.InitWithDefault(ctx, rortracer.WithTimeout(time.Second*5))
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rortracer.Shutdown(shutdownCtx)
	}()

	// Initialize token storage for ror-auth (used by the OIDC signer).
	tokenstoragehelper.Init(vaulttokenadapter.NewVaultStorageAdapter(apiconnections.VaultClient, rorconfig.GetString("TOKEN_STORE_VAULT_PATH")))

	// OIDC: build a single shared validator used by both the auth middleware and
	// the token service, so issuer discovery is performed only once instead of
	// per consumer. Issuers are loaded asynchronously with retry so a slow or
	// unreachable IdP does not block startup, and additional issuers can be added
	// at runtime via the validator. During startup /health/ready is held unready
	// until all issuers have loaded or oidcStartupReadyTimeout elapses.
	oidcValidator, err := oidchelper.NewMultiIssuerValidator()
	if err != nil {
		rlog.Fatal("could not initialize OIDC validator", err)
	}
	oidcConfigs, err := oidchelper.LoadFromEnv()
	if err != nil {
		rlog.Fatal("could not load OIDC configuration", err)
	}
	oidcValidator.LoadIssuersForStartup(ctx, oidcStartupReadyTimeout, oidcConfigs...)

	// Initialize token service with the shared validator and a signer.
	signerIssuer := rorconfig.GetString(rorconfig.OIDC_SIGNER_ISSUER)
	manager := oidchelper.NewManagerWithValidator(signerIssuer, tokenstoragehelper.GetSigningTokenKeyStorage(), oidcValidator)
	tokenservice.SetManager(manager)

	// Register authentication providers (shared OIDC validator + api keys) before
	// the web server starts serving requests.
	authmiddleware.RegisterAuthProvider(oauthmiddleware.NewOauthMiddleware(oidcValidator))
	authmiddleware.RegisterAuthProvider(apikeyauth.NewApiKeyAuthProvider())

	webserver.StartListening(ctx, &wg)

	if apiconnections.RabbitMQConnection.Ping() {
		// Use of unimplemented code
		switchboard.PublishStarted(ctx)
	}

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
