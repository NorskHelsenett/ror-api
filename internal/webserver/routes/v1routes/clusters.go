package v1routes

import (
	"github.com/NorskHelsenett/ror-api/internal/controllers/clusterscontroller"
	"github.com/NorskHelsenett/ror-api/internal/controllers/m2m/configurationcontroller"
	"github.com/gin-gonic/gin"
)

func v1clustersLoginRoute(clusterloginRoute *gin.RouterGroup) {
	clusterloginRoute.POST("/:clusterid/login", clusterscontroller.GetKubeconfig())
}

func v1ClustersRoutes(v1 *gin.RouterGroup) {
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
		clustersRoute.GET("/:clusterid/configs/:name", configurationcontroller.GetTaskConfiguration())

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
}
