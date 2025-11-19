package webserver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apikeyauth"
	"github.com/NorskHelsenett/ror-api/internal/routes"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware/oauthmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/corsmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/headersmiddleware"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"

	"github.com/NorskHelsenett/ror/pkg/telemetry/metric"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func StartListening(ctx context.Context, wg *sync.WaitGroup){
	wg.Go(func() {
		initHttpServer(ctx, wg)
	})	
}

func initHttpServer(ctx context.Context, wg *sync.WaitGroup) error {
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

	router.Use(rlog.LogMiddleware())
	router.Use(metric.MetricMiddleware("/metrics"))
	router.Use(headersmiddleware.HeadersMiddleware())
	router.Use(corsmiddleware.CORS())

	err := router.SetTrustedProxies([]string{"127.0.0.1"})
	if err != nil {
		rlog.Error("could not set trusted proxies", err)
		return err
	}

	routes.SetupRoutes(router)

	httpEndpoint := fmt.Sprintf("%s:%s", rorconfig.GetString(rorconfig.HTTP_HOST), rorconfig.GetString(rorconfig.HTTP_PORT))
	
	httpServ := &http.Server{
		Addr:    httpEndpoint,
		Handler: router,
	}

	wg.Go(func() {
		httpServ.ListenAndServe()
	})

	<-ctx.Done()

	rlog.Info("shutting down http server gracefully")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = httpServ.Shutdown(ctx)
	if err != nil {
		rlog.Error("error while http server was shutting down", err)
		return err
	}
	rlog.Info("http server successfully shut down")

	return nil
}
