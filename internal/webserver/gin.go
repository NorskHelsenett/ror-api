package webserver

import (
	"strings"

	"github.com/NorskHelsenett/ror-api/internal/apiconfig"
	"github.com/NorskHelsenett/ror-api/internal/routes"

	"github.com/NorskHelsenett/ror/pkg/config/configconsts"

	"github.com/NorskHelsenett/ror/pkg/telemetry/metric"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/pprof"
	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func InitHttpServer() {
	useCors := viper.GetBool(configconsts.GIN_USE_CORS)
	allowOrigins := viper.GetString(configconsts.GIN_ALLOW_ORIGINS)
	rlog.Info("Starting web server", rlog.Any("useCors", useCors), rlog.Any("allowedOrigins", allowOrigins))

	router := gin.New()
	if viper.GetBool(configconsts.PROFILER_ENABLED) {
		rlog.Debug("profiler enabled")
		pprof.Register(router)
	}

	router.Use(gzip.Gzip(gzip.DefaultCompression, gzip.WithExcludedPaths([]string{"/metrics"})))
	router.Use(gin.Recovery())
	if viper.GetBool(configconsts.ENABLE_TRACING) {
		router.Use(otelgin.Middleware("ror-api"))
	}

	router.Use(rlog.LogMiddleware())
	router.Use(metric.MetricMiddleware("/metrics"))
	router.Use(headersMiddleware())

	if useCors {
		corsConfig := cors.DefaultConfig()
		//corsConfig.AllowCredentials = true

		origins := strings.Split(allowOrigins, ";")
		corsConfig.AllowOrigins = origins

		corsConfig.AddAllowHeaders("authorization")
		router.Use(cors.New(corsConfig))
	}
	_ = router.SetTrustedProxies([]string{"localhost"})
	routes.SetupRoutes(router)
	rlog.Fatal("router failing", router.Run(apiconfig.GetHTTPEndpoint()))
}

func headersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("x-ror-version", apiconfig.RorVersion.GetVersion())
		c.Header("x-ror-libver", apiconfig.RorVersion.GetLibVer())
		c.Next()
	}
}
