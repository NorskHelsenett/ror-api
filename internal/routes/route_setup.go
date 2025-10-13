package routes

import (
	"time"

	"github.com/NorskHelsenett/ror-api/internal/auth"
	"github.com/NorskHelsenett/ror-api/internal/controllers/aclcontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/apikeyscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/auditlogscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/clusterscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/datacenterscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/desiredversioncontroller"
	healthcontroller "github.com/NorskHelsenett/ror-api/internal/controllers/healthcontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/infocontroller"
	ctrlM2mConfiguration "github.com/NorskHelsenett/ror-api/internal/controllers/m2m/configurationcontroller"
	ctrlM2mEaster "github.com/NorskHelsenett/ror-api/internal/controllers/m2m/eastercontroller"
	ctrlMetrics "github.com/NorskHelsenett/ror-api/internal/controllers/metricscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/notinusecontroller"
	ctrlOperatorConfigs "github.com/NorskHelsenett/ror-api/internal/controllers/operatorconfigscontroller"
	ctrlOrder "github.com/NorskHelsenett/ror-api/internal/controllers/ordercontroller"
	ctrlPrices "github.com/NorskHelsenett/ror-api/internal/controllers/pricescontroller"
	ctrlProjects "github.com/NorskHelsenett/ror-api/internal/controllers/projectscontroller"
	ctrlProviders "github.com/NorskHelsenett/ror-api/internal/controllers/providerscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/resourcescontroller"
	ctrlRulesets "github.com/NorskHelsenett/ror-api/internal/controllers/rulesetscontroller"
	ctrlTasks "github.com/NorskHelsenett/ror-api/internal/controllers/taskscontroller"
	ctrlUsers "github.com/NorskHelsenett/ror-api/internal/controllers/userscontroller"
	v2resourcescontroller "github.com/NorskHelsenett/ror-api/internal/controllers/v2/resourcescontroller"
	viewcontroller "github.com/NorskHelsenett/ror-api/internal/controllers/v2/viewcontroller"
	ctrlWorkspaces "github.com/NorskHelsenett/ror-api/internal/controllers/workspacescontroller"
	"github.com/NorskHelsenett/ror-api/internal/webserver/ratelimiter"

	"github.com/NorskHelsenett/ror-api/pkg/handlers/ssehandler"
	"github.com/NorskHelsenett/ror-api/pkg/middelware/ssemiddleware"

	"github.com/NorskHelsenett/ror-api/internal/controllers/v2/handlerv2selfcontroller"
	"github.com/NorskHelsenett/ror-api/internal/models"
	"github.com/NorskHelsenett/ror-api/internal/webserver/middlewares"
	"github.com/NorskHelsenett/ror-api/internal/webserver/sse"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror-api/internal/docs"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var (
	resourceV1RateLimiter = ratelimiter.NewNamedRorRateLimiter("/v1/resource", 5000, 10000)
	resourceV2RateLimiter = ratelimiter.NewNamedRorRateLimiter("/v2/resource", 50, 100)
)

func SetupRoutes(router *gin.Engine) {

	timeoutduration, err := time.ParseDuration(rorconfig.GetString(rorconfig.HTTP_TIMEOUT))
	if err != nil {
		rlog.Error("Could not parse timeout duration", err)
		timeoutduration = 15 * time.Second
	}

	v1 := router.Group("/v1")
	{
		eventsRoute := v1.Group("events", auth.AuthenticationMiddleware)
		{
			eventsRoute.GET("listen", middlewares.HeadersMiddleware(), sse.Server.HandleSSE())
			eventsRoute.POST("send", sse.Server.Send())
		}
		clusterloginRoute := v1.Group("clusters")
		{
			logintimeoutduration := 120 * time.Second
			clusterloginRoute.Use(middlewares.TimeoutMiddleware(logintimeoutduration))
			clusterloginRoute.Use(auth.AuthenticationMiddleware)
			clusterloginRoute.POST("/:clusterid/login", clusterscontroller.GetKubeconfig())
		}

		v1.Use(middlewares.TimeoutMiddleware(timeoutduration))
		// allow anonymous, for self registrering of agents
		v1.POST("/clusters/register", apikeyscontroller.CreateForAgent())
		infoRoute := v1.Group("/info")
		{
			infoRoute.GET("/version", infocontroller.GetVersion())
		}
		m2mRoute := v1.Group("/m2m")
		{
			// Move along Nothing to see here
			m2mRoute.GET("/", ctrlM2mEaster.RegisterM2m())
			m2mRoute.POST("/heartbeat", notinusecontroller.NotInUse())
			// Allow anonymous POST requests
		}

		v1.Use(auth.AuthenticationMiddleware)
		aclRoute := v1.Group("/acl")
		{
			aclRoute.POST("", aclcontroller.Create())
			aclRoute.PUT("/:id", aclcontroller.Update())
			aclRoute.DELETE("/:id", aclcontroller.Delete())
			aclRoute.GET("/:id", aclcontroller.GetById())

			aclRoute.HEAD("/:scope/:subject/:access", aclcontroller.CheckAcl())

			aclRoute.HEAD("/access/:scope/:subject/:access", aclcontroller.CheckAcl())
			//			aclRoute.GET("/access/:scope/:subject/", aclcontroller.CheckAcl()) // /api/acl/cluster/sdi-ror-dev-32342
			//			aclRoute.GET("/access/:scope/", aclcontroller.CheckAcl())          // /api/acl/cluster
			aclRoute.POST("/filter", aclcontroller.GetByFilter())
			aclRoute.GET("/migrate", aclcontroller.MigrateAcls())
			aclRoute.GET("/scopes", aclcontroller.GetScopes())
		}

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

		clustersRoute := v1.Group("clusters")
		{
			clustersRoute.GET("/:clusterid", clusterscontroller.ClusterGetById())
			clustersRoute.GET("/:clusterid/exists", clusterscontroller.ClusterExistsById())
			clustersRoute.PATCH("/:clusterid/metadata", clusterscontroller.UpdateMetadata())

			clustersRoute.GET("/:clusterid/views/policyreports", clusterscontroller.PolicyreportsView())
			clustersRoute.GET("/:clusterid/views/vulnerabilityreports", clusterscontroller.VulnerabilityReportsView())
			clustersRoute.GET("/:clusterid/views/compliancereports", clusterscontroller.ComplianceReports())
			clustersRoute.GET("/:clusterid/views/ingresses", clusterscontroller.DummyView())
			clustersRoute.GET("/:clusterid/views/nodes", clusterscontroller.DummyView())
			clustersRoute.GET("/:clusterid/views/applications", clusterscontroller.DummyView())
			clustersRoute.GET("/:clusterid/configs/:name", ctrlM2mConfiguration.GetTaskConfiguration())

			clustersRoute.GET("/views/policyreports", clusterscontroller.PolicyreportSummaryView())
			clustersRoute.GET("/views/vulnerabilityreports/byid/:cveid", clusterscontroller.VulnerabilityReportsViewById())
			clustersRoute.GET("/views/vulnerabilityreports/byid", clusterscontroller.GlobalVulnerabilityReportsViewById())
			clustersRoute.GET("/views/vulnerabilityreports", clusterscontroller.VulnerabilityReportsGlobal())
			clustersRoute.GET("/views/compliancereports", clusterscontroller.ComplianceReportsGlobal())
			clustersRoute.POST("/filter", clusterscontroller.ClusterByFilter())
			clustersRoute.POST("/heartbeat", clusterscontroller.RegisterHeartbeat())
			clustersRoute.GET("/metadata", clusterscontroller.GetMetadata())

			clustersRoute.GET("/views/errorlist", clusterscontroller.DummyView())
			clustersRoute.GET("/views/clusterlist", clusterscontroller.DummyView())

			clustersRoute.GET("/self", clusterscontroller.GetSelf())

			clustersRoute.POST("/workspace/:workspaceId/filter", clusterscontroller.ClusterGetByWorkspaceId())
			clustersRoute.GET("/controlplanesMetadata", clusterscontroller.GetControlPlanesMetadata())

			clustersRoute.POST("", clusterscontroller.CreateCluster())
		}

		configsRoute := v1.Group("configs")
		{
			configsRoute.GET("operator", ctrlM2mConfiguration.GetOperatorConfiguration())
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
			ordersRoute.POST("/cluster", ctrlOrder.OrderCluster())
			ordersRoute.DELETE("/cluster", ctrlOrder.DeleteCluster())
			ordersRoute.GET("", ctrlOrder.GetOrders())
			ordersRoute.GET("/:uid", ctrlOrder.GetOrder())
			ordersRoute.DELETE("/:uid", ctrlOrder.DeleteOrder())
		}

		metricsRoute := v1.Group("metrics")
		{
			metricsRoute.GET("", ctrlMetrics.GetTotalByUser())
			metricsRoute.POST("", ctrlMetrics.RegisterResourceMetricsReport())

			metricsRoute.GET("/datacenters", ctrlMetrics.GetForDatacenters())
			metricsRoute.GET("/datacenter/:datacenterId", ctrlMetrics.GetByDatacenterId())

			metricsRoute.GET("/clusters", ctrlMetrics.GetForClusters())
			metricsRoute.GET("/clusters/workspace/:workspaceId", ctrlMetrics.GetForClustersByWorkspaceId())
			metricsRoute.GET("/cluster/:clusterId", ctrlMetrics.GetByClusterId())

			metricsRoute.GET("/custom/cluster/:property", ctrlMetrics.MetricsForClustersByProperty())

			metricsRoute.GET("/total", ctrlMetrics.GetTotal())

			metricsRoute.GET("/workspace/:workspaceId", ctrlMetrics.GetByWorkspaceId())
			metricsRoute.POST("/workspaces/filter", ctrlMetrics.GetForWorkspaces())
			metricsRoute.POST("/workspaces/datacenter/:datacenterId/filter", ctrlMetrics.GetForWorkspacesByDatacenterId())
		}

		operatorconfigRoute := v1.Group("operatorconfigs")
		{
			operatorconfigRoute.GET("", ctrlOperatorConfigs.GetAll())
			operatorconfigRoute.GET("/:id", ctrlOperatorConfigs.GetById())
			operatorconfigRoute.POST("", ctrlOperatorConfigs.Create())
			operatorconfigRoute.PUT("/:id", ctrlOperatorConfigs.Update())
			operatorconfigRoute.DELETE("/:id", ctrlOperatorConfigs.Delete())
		}

		providerRouter := v1.Group("providers")
		{
			providerRouter.GET("", ctrlProviders.GetAll())
			providerRouter.GET("/:providerType/kubernetes/versions", ctrlProviders.GetKubernetesVersionByProvider())
		}

		pricesRoute := v1.Group("prices")
		{
			pricesRoute.GET("", ctrlPrices.GetAll())

			pricesRoute.GET("/:priceId", ctrlPrices.GetById())
			pricesRoute.POST("", ctrlPrices.Create(), middlewares.AuditLogMiddleware("Price created", models.AuditCategoryPrice, models.AuditActionCreate))
			pricesRoute.PUT("/:priceId", ctrlPrices.Update(), middlewares.AuditLogMiddleware("Price updated", models.AuditCategoryPrice, models.AuditActionUpdate))
			// pricesRoute.DELETE(":id", ctrlPrices.Delete(), middlewares.AuditLogMiddleware("Price deleted", models.Price.String(), models.DELETE.String()))

			pricesRoute.GET("/provider/:providerName", ctrlPrices.GetByProvider())
		}

		projectsRoute := v1.Group("projects")
		{
			projectsRoute.GET("/:id", ctrlProjects.GetById())
			projectsRoute.GET("/:id/clusters", ctrlProjects.GetClustersByProjectId())

			projectsRoute.POST("/filter", ctrlProjects.GetByFilter())

			projectsRoute.POST("", ctrlProjects.Create())
			projectsRoute.PUT("/:id", ctrlProjects.Update())
			projectsRoute.DELETE(":id", ctrlProjects.Delete())
		}

		resourceRoute := v1.Group("resources")
		resourceRoute.Use(resourceV1RateLimiter.RateLimiter)
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
			selfRoute.GET("", ctrlUsers.GetUser())
			selfRoute.POST("/apikeys", ctrlUsers.CreateApikey())
			selfRoute.POST("/apikeys/filter", ctrlUsers.GetApiKeysByFilter())
			selfRoute.DELETE("/apikeys/:id", ctrlUsers.DeleteApiKey())
		}

		tasksRoute := v1.Group("tasks")
		{
			tasksRoute.GET("", ctrlTasks.GetAll())
			tasksRoute.GET("/:id", ctrlTasks.GetById())
			tasksRoute.POST("", ctrlTasks.Create())
			tasksRoute.PUT("/:id", ctrlTasks.Update())
			tasksRoute.DELETE("", ctrlTasks.Delete())
		}

		workspacesRoute := v1.Group("workspaces")
		{
			workspacesRoute.GET("", ctrlWorkspaces.GetAll())
			workspacesRoute.GET("/:workspaceName", ctrlWorkspaces.GetByName())
			workspacesRoute.GET("/id/:id", ctrlWorkspaces.GetById())
			workspacesRoute.PUT("/:id", ctrlWorkspaces.Update())
			workspacesRoute.POST("/:workspaceName/login", ctrlWorkspaces.GetKubeconfig())
		}

		rulesetsRoute := v1.Group("rulesetsController")
		{
			if rorconfig.GetBool(rorconfig.DEVELOPMENT) {
				rulesetsRoute.GET("", ctrlRulesets.GetAll())
			}

			rulesetsRoute.GET("/cluster/:clusterId", ctrlRulesets.GetByCluster())
			rulesetsRoute.GET("/internal", ctrlRulesets.GetInternal())

			rulesetsRoute.PUT("/:rulesetId/resources", ctrlRulesets.AddResource())

			rulesetsRoute.DELETE("/:rulesetId/resources/:resourceId", ctrlRulesets.DeleteResource())

			rulesetsRoute.POST("/:rulesetId/resources/:resourceId/rules", ctrlRulesets.AddResourceRule())
			rulesetsRoute.DELETE("/:rulesetId/resources/:resourceId/rules/:ruleId", ctrlRulesets.DeleteResourceRule())

		}
	}

	v2 := router.Group("/v2")

	eventsRoute := v2.Group("events", auth.AuthenticationMiddleware)
	{
		eventstimeout := 60 * time.Second
		eventsRoute.GET("listen", ssemiddleware.SSEHeadersMiddleware(), ssehandler.HandleSSE())
		eventsRoute.POST("send", middlewares.TimeoutMiddleware(eventstimeout), ssehandler.Send())
	}

	v2.Use(auth.AuthenticationMiddleware)
	v2.Use(middlewares.TimeoutMiddleware(timeoutduration))
	// Self
	selfv2Route := v2.Group("self")
	selfv2Route.GET("", handlerv2selfcontroller.GetSelf())
	selfv2Route.POST("/apikeys", handlerv2selfcontroller.CreateOrRenewApikey())
	selfv2Route.DELETE("/apikeys/:id", handlerv2selfcontroller.DeleteApiKey())

	router.GET("/health", healthcontroller.GetHealthStatus())
	router.GET("/metrics", gin.WrapH(promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{EnableOpenMetrics: true})))

	docs.SwaggerInfo.BasePath = "/"
	docs.SwaggerInfo.Version = rorversion.GetRorVersion().GetVersion()
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	resourceRoute := v2.Group("resources")
	resourceRoute.Use(resourceV2RateLimiter.RateLimiter)
	resourceRoute.GET("", v2resourcescontroller.GetResources())
	resourceRoute.POST("", v2resourcescontroller.NewResource())
	resourceRoute.DELETE("/uid/:uid", v2resourcescontroller.DeleteResource())
	resourceRoute.HEAD("/uid/:uid", v2resourcescontroller.ExistsResources())
	resourceRoute.GET("/hashes", v2resourcescontroller.GetResourceHashList())

	//deprecated: let client deal with special cases
	resourceRoute.GET("/uid/:uid", v2resourcescontroller.GetResource())
	resourceRoute.PUT("/uid/:uid", v2resourcescontroller.UpdateResource())

	viewsRoute := v2.Group("views")
	{
		viewsRoute.GET("", viewcontroller.GetViews())
		viewsRoute.GET("/:viewid", viewcontroller.GetView())
	}

}
