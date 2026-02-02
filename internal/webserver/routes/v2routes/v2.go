package v2routes

import (
	"time"

	"github.com/NorskHelsenett/ror-api/internal/controllers/apikeyscontroller/v2"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/resourcescontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/tokencontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/viewcontroller"

	"github.com/NorskHelsenett/ror-api/pkg/handlers/healthginhandler"
	"github.com/NorskHelsenett/ror-api/pkg/handlers/ssehandler"

	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/rorratelimiter"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/ssemiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/timeoutmiddleware"

	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/handlerv2selfcontroller"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror-api/internal/docs"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
)

var (
	resourceV2rorratelimiter = rorratelimiter.NewNamedRorRateLimiter("/v2/resource", 50, 100)
	defaultV1Timeout         = 15 * time.Second
	timeoutduration          time.Duration
)

func SetupRoutes(router *gin.Engine) error {
	var err error
	timeoutduration, err = time.ParseDuration(rorconfig.GetString(rorconfig.HTTP_TIMEOUT))
	if err != nil {
		rlog.Warn("Could not parse timeout, defaulting to defaultTimeout", rlog.String("error", err.Error()))
		timeoutduration = defaultV1Timeout
	}

	// V2 Events no default timeout in SSE
	v2eventsRoute := router.Group("/v2/events", authmiddleware.AuthenticationMiddleware)
	setupV2EventsRoute(v2eventsRoute)

	// apikeys register agent . unauthenticated
	router.POST("/v2/apikeys/register/agent", apikeyscontroller.RegisterAgent())

	// V2 routes
	v2 := router.Group("/v2",
		timeoutmiddleware.TimeoutMiddleware(timeoutduration),
		authmiddleware.AuthenticationMiddleware,
	)

	// Self
	selfv2Route := v2.Group("self")
	selfv2Route.GET("", handlerv2selfcontroller.GetSelf())
	selfv2Route.POST("/apikeys", handlerv2selfcontroller.CreateOrRenewApikey())
	selfv2Route.DELETE("/apikeys/:id", handlerv2selfcontroller.DeleteApiKey())

	router.GET("/health", healthginhandler.GetGinHandler())
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true})))

	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Version = rorversion.GetRorVersion().GetVersion()
	router.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	resourceRoute := v2.Group("resources")
	resourceRoute.Use(resourceV2rorratelimiter.RateLimiter)
	resourceRoute.GET("", resourcescontroller.GetResources())
	resourceRoute.POST("", resourcescontroller.NewResource())
	resourceRoute.DELETE("/uid/:uid", resourcescontroller.DeleteResource())
	resourceRoute.HEAD("/uid/:uid", resourcescontroller.ExistsResources())
	resourceRoute.GET("/hashes", resourcescontroller.GetResourceHashList())

	//deprecated: let client deal with special cases
	resourceRoute.GET("/uid/:uid", resourcescontroller.GetResource())
	resourceRoute.PUT("/uid/:uid", resourcescontroller.UpdateResource())

	viewsRoute := v2.Group("views")
	{
		viewsRoute.GET("", viewcontroller.GetViews())
		viewsRoute.GET("/:viewid", viewcontroller.GetView())
	}
	tokenroute := v2.Group("/token")
	{
		tokenroute.POST("/exchange", tokencontroller.ExchangeToken())
	}
	return nil
}

func setupV2EventsRoute(v2eventsRoute *gin.RouterGroup) {
	v2eventsRoute.GET("listen", ssemiddleware.SSEHeadersMiddlewareV2(), ssehandler.HandleSSE())
	v2eventsRoute.POST("send", timeoutmiddleware.TimeoutMiddleware(timeoutduration), ssehandler.Send())
}
