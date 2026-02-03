package utilityroutes

import (
	"github.com/NorskHelsenett/ror-api/pkg/handlers/healthginhandler"

	"github.com/NorskHelsenett/ror/pkg/config/rorversion"

	"github.com/NorskHelsenett/ror-api/internal/docs"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
)

func SetupRoutes(router *gin.Engine) error {

	router.GET("/health", healthginhandler.GetGinHandler())
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true})))

	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Version = rorversion.GetRorVersion().GetVersion()
	router.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	return nil
}
