package webserver

import (
	"fmt"
	"os"

	"github.com/NorskHelsenett/ror-api/internal/apikeyauth"
	"github.com/NorskHelsenett/ror-api/internal/routes"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware/oauthprovider"
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

func StartListening(sigs chan os.Signal, done chan struct{}) {
	go func(sigs chan os.Signal, done chan struct{}) {
		InitHttpServer()
		<-sigs
		done <- struct{}{}
	}(sigs, done)
}

func InitHttpServer() {

	authmiddleware.RegisterAuthProvider(oauthprovider.NewOauthProvider())
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

	_ = router.SetTrustedProxies([]string{"localhost"})
	routes.SetupRoutes(router)
	rlog.Fatal("router failing", router.Run(getHTTPEndpoint()))
}
func getHTTPEndpoint() string {
	return fmt.Sprintf("%s:%s", rorconfig.GetString(rorconfig.HTTP_HOST), rorconfig.GetString(rorconfig.HTTP_PORT))
}
