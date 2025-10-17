package webserver

import (
	"github.com/NorskHelsenett/ror-api/internal/apiconfig"
	"github.com/NorskHelsenett/ror-api/internal/routes"
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

func InitHttpServer() {
	useCors := rorconfig.GetBool(rorconfig.GIN_USE_CORS)
	allowOrigins := rorconfig.GetString(rorconfig.GIN_ALLOW_ORIGINS)
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
	rlog.Fatal("router failing", router.Run(apiconfig.GetHTTPEndpoint()))
}
