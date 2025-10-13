package webserver

import (
	"fmt"
	"os"
	"strings"

	"github.com/NorskHelsenett/ror-api/internal/routes"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"

	"github.com/NorskHelsenett/ror/pkg/telemetry/metric"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-contrib/cors"
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
	rlog.Fatal("router failing", router.Run(getHTTPEndpoint()))
}

func headersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("x-ror-version", rorversion.GetRorVersion().GetVersion())
		c.Header("x-ror-libver", rorversion.GetRorVersion().GetLibVer())
		c.Next()
	}
}

func getHTTPEndpoint() string {
	return fmt.Sprintf("%s:%s", rorconfig.GetString(rorconfig.HTTP_HOST), rorconfig.GetString(rorconfig.HTTP_PORT))
}
