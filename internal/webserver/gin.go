package webserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apikeyauth"
	"github.com/NorskHelsenett/ror-api/internal/webserver/routes/utilityroutes"
	"github.com/NorskHelsenett/ror-api/internal/webserver/routes/v1routes"
	"github.com/NorskHelsenett/ror-api/internal/webserver/routes/v2routes"

	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware/oauthmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/corsmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/headersmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/metricsmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/rlogmiddleware"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

func StartListening(ctx context.Context, wg *sync.WaitGroup) {
	wg.Go(func() {
		err := startHttpServer(ctx)
		if err != nil {
			rlog.Fatal("the ror api encountered an unexpected error: ", err)
			return
		}
	})
}

func startHttpServer(ctx context.Context) error {

	if !rorconfig.GetBool(rorconfig.DEVELOPMENT) {
		gin.SetMode(gin.ReleaseMode)
	}

	authmiddleware.RegisterAuthProvider(oauthmiddleware.NewDefaultOauthMiddleware())
	authmiddleware.RegisterAuthProvider(apikeyauth.NewApiKeyAuthProvider())

	useCors := rorconfig.GetBool(rorconfig.HTTP_USE_CORS)
	allowOrigins := rorconfig.GetString(rorconfig.HTTP_ALLOW_ORIGINS)

	rlog.Info("Starting web server", rlog.Any("useCors", useCors), rlog.Any("allowedOrigins", allowOrigins))

	router := gin.New()

	if rorconfig.GetBool(rorconfig.PROFILER_ENABLED) {
		rlog.Debug("profiler enabled")
		pprof.Register(router)
	}

	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/metrics"})))
	router.Use(gin.Recovery())
	if rorconfig.GetBool(rorconfig.ENABLE_TRACING) {
		router.Use(otelgin.Middleware("ror-api"))
	}

	router.Use(rlogmiddleware.LogMiddleware())
	router.Use(metricsmiddleware.MetricMiddleware("/metrics"))
	router.Use(headersmiddleware.HeadersMiddleware())
	router.Use(corsmiddleware.CORS())

	err := router.SetTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		rlog.Error("could not set trusted proxies", err)
		return err
	}
	err = v1routes.SetupRoutes(router)
	if err != nil {
		rlog.Error("could not setup v1 routes", err)
		return err
	}
	err = v2routes.SetupRoutes(router)
	if err != nil {
		rlog.Error("could not setup v2 routes", err)
		return err
	}

	err = utilityroutes.SetupRoutes(router)
	if err != nil {
		rlog.Error("could not setup utility routes", err)
		return err
	}

	httpEndpoint := fmt.Sprintf("%s:%s", rorconfig.GetString(rorconfig.HTTP_HOST), rorconfig.GetString(rorconfig.HTTP_PORT))

	httpServ := &http.Server{
		Addr:              httpEndpoint,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	chanHttpErr := make(chan error)

	go func() {
		err = httpServ.ListenAndServe()
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				chanHttpErr <- err
			}
		}
	}()

	select {
	// if we are toldt to abort, initate gracefull shutdown of the http
	// server
	case <-ctx.Done():
		rlog.Info("http server: attempting graceful shutdown")

		// we create a new context here because the one passed to use is
		// canceled in this case
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err = httpServ.Shutdown(ctx)
		if err != nil {
			rlog.Error("error while http server was shutting down", err)
			return err
		}

		rlog.Info("http server: graceful shutdown complete")
		return nil
	// handle unexpected errors from the http server
	case <-chanHttpErr:
		rlog.Error("http server closed unexpectedly", err)
		return err
	}
}
