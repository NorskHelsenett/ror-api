package apikeyscontroller

import (
	"context"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/apikeysservice"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/apikeystypes/v2"
	"github.com/gin-gonic/gin"
)

// Register an agent.
// Identity must be authorized to register an agent?
//
//	@Summary	Register an agent
//	@Schemes
//	@Description	Register a cluster agent.
//	@Tags			apikeys
//	@Accept			application/json
//	@Produce		application/json
//	@Param			data	body		apikeystypes.RegisterClusterRequest	true	"data"
//	@Success		200		{object}	apikeystypes.RegisterClusterResponse
//	@Failure		403		{object}	rorerror.ErrorData
//	@Failure		400		{object}	rorerror.ErrorData
//	@Failure		500		{string}	Failure	message
//	@Router			/v2/apikeys/agent/register [post]
//	@Security		ApiKey || AccessToken
func RegisterAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithCancel(c.Request.Context())
		defer cancel()

		// // Access check
		// // Scope: ror
		// // Subject: cluster
		// // Access: create
		// accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectCluster)
		// accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		// if !accessObject.Create {
		// 	rerr := rorginerror.NewRorGinError(http.StatusForbidden, "No access")
		// 	rerr.GinLogErrorAbort(c)
		// 	return
		// }

		var req apikeystypes.RegisterClusterRequest
		if err := c.BindJSON(&req); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		resp, err := apikeysservice.CreateForAgentV2(ctx, &req)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not register agent", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, resp)

		//clusterId, apiKey, err := clustersservice.RegisterCluster(ctx, req.ClusterId)
	}
}
