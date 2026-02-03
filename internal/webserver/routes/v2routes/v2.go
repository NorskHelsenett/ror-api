package v2routes

import (
	"time"

	"github.com/NorskHelsenett/ror-api/internal/controllers/apikeyscontroller/v2"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/resourcescontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/tokencontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/viewcontroller"

	"github.com/NorskHelsenett/ror-api/pkg/handlers/ssehandler"

	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/rorratelimiter"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/ssemiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/timeoutmiddleware"

	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/handlerv2selfcontroller"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

var (
	resourceV2rorratelimiter = rorratelimiter.NewNamedRorRateLimiter("/v2/resource", 50, 100)
	defaultV2Timeout         = 15 * time.Second
	timeoutduration          time.Duration
)

func SetupRoutes(router *gin.Engine) error {
	var err error
	timeoutduration, err = time.ParseDuration(rorconfig.GetString(rorconfig.HTTP_TIMEOUT))
	if err != nil {
		rlog.Warn("Could not parse timeout, defaulting to defaultTimeout", rlog.String("error", err.Error()))
		timeoutduration = defaultV2Timeout
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

	setupV2ResourcesRoute(v2)

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

func setupV2ResourcesRoute(v2 *gin.RouterGroup) {
	resourceRoute := v2.Group("resources")
	resourceRoute.Use(resourceV2rorratelimiter.RateLimiter)
	resourceRoute.GET("", resourcescontroller.GetResources())
	resourceRoute.POST("", resourcescontroller.NewResource())
	resourceRoute.GET("/hashes", resourcescontroller.GetResourceHashList())
	resourceRoute.GET("/uid/:uid", resourcescontroller.GetResource())
	resourceRoute.PUT("/uid/:uid", resourcescontroller.UpdateResource())
	resourceRoute.DELETE("/uid/:uid", resourcescontroller.DeleteResource())
	resourceRoute.HEAD("/uid/:uid", resourcescontroller.ExistsResources())
}

func setupV2EventsRoute(v2eventsRoute *gin.RouterGroup) {
	v2eventsRoute.GET("listen", ssemiddleware.SSEHeadersMiddlewareV2(), ssehandler.HandleSSE())
	v2eventsRoute.POST("send", timeoutmiddleware.TimeoutMiddleware(timeoutduration), ssehandler.Send())
}
