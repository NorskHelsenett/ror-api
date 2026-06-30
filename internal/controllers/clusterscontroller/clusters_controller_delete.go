package clusterscontroller

import (
	"errors"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/acl/aclservice"
	"github.com/NorskHelsenett/ror-api/internal/apiservices/clustersservice"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"
	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

// Purge a cluster and all of its related data by cluster uid.
// Deletes the cluster document, its v1 resources, its resourcesv2 tree and its acl entries.
// Requires ror global delete access.
//
//	@Summary	Purge a cluster by uid
//	@Schemes
//	@Description	Delete a cluster and all of its related data (resources, resourcesv2, acl) by uid
//	@Tags			clusters
//	@Accept			application/json
//	@Produce		application/json
//	@Param			uid	path		string	true	"cluster uid"
//	@Success		200	{object}	clustersservice.PurgeResult
//	@Failure		403	{string}	Forbidden
//	@Failure		401	{object}	rorerror.ErrorData
//	@Failure		404	{string}	NotFound
//	@Failure		500	{string}	Failure	message
//	@Router			/v1/clusters/uid/{uid} [delete]
//	@Security		ApiKey || AccessToken
func DeleteClusterByUid() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		uid := c.Param("uid")
		if uid == "" {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing uid")
			rerr.GinLogErrorAbort(c)
			return
		}

		// Access check
		// Scope: ror
		// Subject: global
		// Access: delete
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Delete {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		result, err := clustersservice.PurgeClusterByUid(ctx, uid)
		if err != nil {
			if errors.Is(err, clustersservice.ErrClusterNotFound) {
				c.JSON(http.StatusNotFound, "404: Cluster not found")
				return
			}
			rlog.Errorc(ctx, "could not purge cluster", err, rlog.String("uid", uid))
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not purge cluster", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}
