package v1routes

import (
	"github.com/NorskHelsenett/ror-api/internal/controllers/aclcontroller"
	"github.com/gin-gonic/gin"
)

func v1aclRoutes(v1 *gin.RouterGroup) {
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
}
