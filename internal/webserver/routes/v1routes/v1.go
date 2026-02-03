package v1routes

import (
	"time"

	"github.com/NorskHelsenett/ror-api/internal/controllers/apikeyscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/auditlogscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/clusterscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/datacenterscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/desiredversioncontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/infocontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/m2m/configurationcontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/m2m/eastercontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/metricscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/operatorconfigscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/ordercontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/pricescontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/projectscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/providerscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/resourcescontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/rulesetscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/taskscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/userscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/tokencontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/workspacescontroller"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/auditmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/authmiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/rorratelimiter"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/ssemiddleware"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/timeoutmiddleware"

	"github.com/NorskHelsenett/ror-api/internal/models"
	"github.com/NorskHelsenett/ror-api/internal/webserver/sse"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

var (
	resourceV1rorratelimiter = rorratelimiter.NewNamedRorRateLimiter("/v1/resource", 5000, 10000)
	defaultV1Timeout         = 15 * time.Second
	logintimeoutduration     = 120 * time.Second
	timeoutduration          time.Duration
)

func SetupRoutes(router *gin.Engine) error {
	var err error
	// Parse timeout duration from config
	timeoutduration, err = time.ParseDuration(rorconfig.GetString(rorconfig.HTTP_TIMEOUT))
	if err != nil {
		rlog.Warn("Could not parse timeout, defaulting to defaultTimeout", rlog.String("error", err.Error()))
		timeoutduration = defaultV1Timeout
	}

	// events route
	// No timeout for SSE connections
	eventsRoute := router.Group("/v1/events",
		authmiddleware.AuthenticationMiddleware,
	)
	v1eventsRoute(eventsRoute)

	// cluster login route
	// Longer timeout for login requests
	clusterloginRoute := router.Group("/v1/clusters",
		timeoutmiddleware.TimeoutMiddleware(logintimeoutduration),
		authmiddleware.AuthenticationMiddleware,
	)
	v1clustersLoginRoute(clusterloginRoute)

	// Anonymous routes with timeout
	anonomousRoutes := router.Group("/v1",
		timeoutmiddleware.TimeoutMiddleware(timeoutduration),
	)
	v1anonymousRoutes(anonomousRoutes)

	// v1 Routes
	v1 := router.Group("/v1",
		timeoutmiddleware.TimeoutMiddleware(timeoutduration),
		authmiddleware.AuthenticationMiddleware,
	)

	v1aclRoutes(v1)

	apikeysRoute := v1.Group("apikeys")
	{
		apikeysRoute.POST("/filter", apikeyscontroller.GetByFilter())
		apikeysRoute.DELETE("/:id", apikeyscontroller.Delete())
		apikeysRoute.POST("", apikeyscontroller.CreateApikey())
	}

	auditlogsRoute := v1.Group("auditlogs")
	{
		auditlogsRoute.GET("/:id", auditlogscontroller.GetById())
		auditlogsRoute.POST("/filter", auditlogscontroller.GetByFilter())
		auditlogsRoute.GET("/metadata", auditlogscontroller.GetMetadata())
	}

	clusterRoute := v1.Group("cluster")
	{
		clusterRoute.GET("/:clusterid", clusterscontroller.ClusterGetById())
		clusterRoute.GET("/:clusterid/exists", clusterscontroller.ClusterExistsById())
		clusterRoute.POST("/:clusterid/heartbeat", clusterscontroller.RegisterHeartbeat())
		clusterRoute.PATCH("/:clusterid/metadata", clusterscontroller.UpdateMetadata())
		clusterRoute.POST("/heartbeat", clusterscontroller.RegisterHeartbeat())
	}

	v1ClustersRoutes(v1)

	configsRoute := v1.Group("configs")
	{
		configsRoute.GET("operator", configurationcontroller.GetOperatorConfiguration())
	}

	datacentersRoute := v1.Group("datacenters")
	{
		datacentersRoute.GET("", datacenterscontroller.GetAll())
		datacentersRoute.GET("/:datacenterName", datacenterscontroller.GetByName())
		datacentersRoute.GET("/id/:id", datacenterscontroller.GetById())
		datacentersRoute.POST("", datacenterscontroller.Create())
		datacentersRoute.PUT("/:datacenterId", datacenterscontroller.Update())
	}

	desiredVersionsRoute := v1.Group("/desired_versions")
	{
		desiredVersionsRoute.GET("", desiredversioncontroller.GetAll())
		desiredVersionsRoute.GET("/:key", desiredversioncontroller.GetByKey())
		desiredVersionsRoute.POST("", desiredversioncontroller.Create())
		desiredVersionsRoute.PUT("/:key", desiredversioncontroller.Update())
		desiredVersionsRoute.DELETE("/:key", desiredversioncontroller.Delete())
	}

	ordersRoute := v1.Group("orders")
	{
		ordersRoute.POST("/cluster", ordercontroller.OrderCluster())
		ordersRoute.DELETE("/cluster", ordercontroller.DeleteCluster())
		ordersRoute.GET("", ordercontroller.GetOrders())
		ordersRoute.GET("/:uid", ordercontroller.GetOrder())
		ordersRoute.DELETE("/:uid", ordercontroller.DeleteOrder())
	}

	metricsRoute := v1.Group("metrics")
	{
		metricsRoute.GET("", metricscontroller.GetTotalByUser())
		metricsRoute.POST("", metricscontroller.RegisterResourceMetricsReport())

		metricsRoute.GET("/datacenters", metricscontroller.GetForDatacenters())
		metricsRoute.GET("/datacenter/:datacenterId", metricscontroller.GetByDatacenterId())

		metricsRoute.GET("/clusters", metricscontroller.GetForClusters())
		metricsRoute.GET("/clusters/workspace/:workspaceId", metricscontroller.GetForClustersByWorkspaceId())
		metricsRoute.GET("/cluster/:clusterId", metricscontroller.GetByClusterId())

		metricsRoute.GET("/custom/cluster/:property", metricscontroller.MetricsForClustersByProperty())

		metricsRoute.GET("/total", metricscontroller.GetTotal())

		metricsRoute.GET("/workspace/:workspaceId", metricscontroller.GetByWorkspaceId())
		metricsRoute.POST("/workspaces/filter", metricscontroller.GetForWorkspaces())
		metricsRoute.POST("/workspaces/datacenter/:datacenterId/filter", metricscontroller.GetForWorkspacesByDatacenterId())
	}

	operatorconfigRoute := v1.Group("operatorconfigs")
	{
		operatorconfigRoute.GET("", operatorconfigscontroller.GetAll())
		operatorconfigRoute.GET("/:id", operatorconfigscontroller.GetById())
		operatorconfigRoute.POST("", operatorconfigscontroller.Create())
		operatorconfigRoute.PUT("/:id", operatorconfigscontroller.Update())
		operatorconfigRoute.DELETE("/:id", operatorconfigscontroller.Delete())
	}

	providerRouter := v1.Group("providers")
	{
		providerRouter.GET("", providerscontroller.GetAll())
		providerRouter.GET("/:providerType/kubernetes/versions", providerscontroller.GetKubernetesVersionByProvider())
	}

	pricesRoute := v1.Group("prices")
	{
		pricesRoute.GET("", pricescontroller.GetAll())

		pricesRoute.GET("/:priceId", pricescontroller.GetById())
		pricesRoute.POST("", pricescontroller.Create(), auditmiddleware.AuditLogMiddleware("Price created", models.AuditCategoryPrice, models.AuditActionCreate))
		pricesRoute.PUT("/:priceId", pricescontroller.Update(), auditmiddleware.AuditLogMiddleware("Price updated", models.AuditCategoryPrice, models.AuditActionUpdate))
		// pricesRoute.DELETE(":id", ctrlPrices.Delete(), middlewares.AuditLogMiddleware("Price deleted", models.Price.String(), models.DELETE.String()))

		pricesRoute.GET("/provider/:providerName", pricescontroller.GetByProvider())
	}

	projectsRoute := v1.Group("projects")
	{
		projectsRoute.GET("/:id", projectscontroller.GetById())
		projectsRoute.GET("/:id/clusters", projectscontroller.GetClustersByProjectId())

		projectsRoute.POST("/filter", projectscontroller.GetByFilter())

		projectsRoute.POST("", projectscontroller.Create())
		projectsRoute.PUT("/:id", projectscontroller.Update())
		projectsRoute.DELETE(":id", projectscontroller.Delete())
	}

	resourceRoute := v1.Group("resources")
	resourceRoute.Use(resourceV1rorratelimiter.RateLimiter)
	{
		resourceRoute.GET("", resourcescontroller.GetResources())
		resourceRoute.POST("", resourcescontroller.NewResource())
		resourceRoute.GET("/uid/:uid", resourcescontroller.GetResource())
		resourceRoute.PUT("/uid/:uid", resourcescontroller.UpdateResource())
		resourceRoute.DELETE("/uid/:uid", resourcescontroller.DeleteResource())
		resourceRoute.HEAD("/uid/:uid", resourcescontroller.ExistsResources())

		resourceRoute.GET("/hashes", resourcescontroller.GetResourceHashList())
	}

	usersRoute := v1.Group("users")
	{
		selfRoute := usersRoute.Group("self")
		selfRoute.GET("", userscontroller.GetUser())
		selfRoute.POST("/apikeys", userscontroller.CreateApikey())
		selfRoute.POST("/apikeys/filter", userscontroller.GetApiKeysByFilter())
		selfRoute.DELETE("/apikeys/:id", userscontroller.DeleteApiKey())
	}

	tasksRoute := v1.Group("tasks")
	{
		tasksRoute.GET("", taskscontroller.GetAll())
		tasksRoute.GET("/:id", taskscontroller.GetById())
		tasksRoute.POST("", taskscontroller.Create())
		tasksRoute.PUT("/:id", taskscontroller.Update())
		tasksRoute.DELETE("", taskscontroller.Delete())
	}

	workspacesRoute := v1.Group("workspaces")
	{
		workspacesRoute.GET("", workspacescontroller.GetAll())
		workspacesRoute.GET("/:workspaceName", workspacescontroller.GetByName())
		workspacesRoute.GET("/id/:id", workspacescontroller.GetById())
		workspacesRoute.PUT("/:id", workspacescontroller.Update())
		workspacesRoute.POST("/:workspaceName/login", workspacescontroller.GetKubeconfig())
	}

	rulesetsRoute := v1.Group("rulesetsController")
	{
		if rorconfig.GetBool(rorconfig.DEVELOPMENT) {
			rulesetsRoute.GET("", rulesetscontroller.GetAll())
		}

		rulesetsRoute.GET("/cluster/:clusterId", rulesetscontroller.GetByCluster())
		rulesetsRoute.GET("/internal", rulesetscontroller.GetInternal())

		rulesetsRoute.PUT("/:rulesetId/resources", rulesetscontroller.AddResource())

		rulesetsRoute.DELETE("/:rulesetId/resources/:resourceId", rulesetscontroller.DeleteResource())

		rulesetsRoute.POST("/:rulesetId/resources/:resourceId/rules", rulesetscontroller.AddResourceRule())
		rulesetsRoute.DELETE("/:rulesetId/resources/:resourceId/rules/:ruleId", rulesetscontroller.DeleteResourceRule())

	}

	return nil
}

func v1anonymousRoutes(anonomousRoutes *gin.RouterGroup) {

	anonomousRoutes.GET("/token/jwks", tokencontroller.GetJwks())
	// allow anonymous, for self registrering of agents
	anonomousRoutes.POST("/clusters/register", apikeyscontroller.CreateForAgent())
	anonomousRoutes.GET("/info/version", infocontroller.GetVersion())
	// Move along Nothing to see here
	anonomousRoutes.GET("/m2m", eastercontroller.RegisterM2m())
}

func v1eventsRoute(eventsRoute *gin.RouterGroup) {
	eventsRoute.GET("listen", ssemiddleware.SSEHeadersMiddlewareV1(), sse.Server.HandleSSE())
	eventsRoute.POST("send", sse.Server.Send())
}
