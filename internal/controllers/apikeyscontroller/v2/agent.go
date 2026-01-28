package apikeyscontroller

import (
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/apicontracts/clustersapi/v2"
	"github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/gin-gonic/gin"
)

// Register an agent.
// Identity must be authorized to register an agent?
//
//	@Summary	Register an agent
//	@Schemes
//	@Description	Register an cluster.
//	@Tags			clusters
//	@Accept			application/json
//	@Produce		application/json
//	@Param			data	body		clustersapi.RegisterClusterRequest	true	"data"
//	@Success		200	{object}	clustersapi.RegisterClusterResponse
//	@Failure		403	{string}	rorerror.ErrorData
//	@Failure		400	{object}	rorerror.ErrorData
//	@Failure		500	{string}	Failure	message
//	@Router			/v2/apikeys/agent/register [post]
//	@Security		ApiKey || AccessToken
func RegisterAgent() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: cluster
		// Access: create
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectCluster)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			rerr := rorginerror.NewRorGinError(http.StatusForbidden, "No access")
			rerr.GinLogErrorAbort(c)
			return
		}

		var req clustersapi.RegisterClusterRequest
		if err := c.BindJSON(&req); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, clustersapi.RegisterClusterResponse{
			ClusterId: req.ClusterId,
			ApiKey:    "dummy-api-key",
		})

		//clusterId, apiKey, err := clustersservice.RegisterCluster(ctx, req.ClusterId)
	}
}
