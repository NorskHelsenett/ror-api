// TODO: Describe package
package auditlogscontroller

import (
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	auditLogService "github.com/NorskHelsenett/ror-api/internal/apiservices/auditlogs"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	validate = validator.New()
}

// GetByFilter returns audit logs filtered by the provided filter.
//
//	@Summary	Get audit logs by filter
//	@Schemes
//	@Description	Get audit logs by filter
//	@Tags			auditlogs
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200						{object}	apicontracts.PaginatedResult[apicontracts.AuditLog]
//	@Failure		403						{object}	rorerror.ErrorData
//	@Failure		400						{object}	rorerror.ErrorData
//	@Failure		401						{object}	rorerror.ErrorData
//	@Failure		500						{object}	rorerror.ErrorData
//	@Router			/v1/auditlogs/filter	[post]
//	@Param			filter					body	apicontracts.Filter	true	"Filter"
//	@Security		ApiKey || AccessToken
func GetByFilter() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: global
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var filter apicontracts.Filter
		if err := c.BindJSON(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not get auditlog", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		result, err := auditLogService.GetByFilter(ctx, &filter)
		if err != nil {
			rlog.Errorc(ctx, "could not get auditlogs", err)
			c.JSON(http.StatusInternalServerError, err)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// GetById returns a single audit log by its id.
//
//	@Summary	Get audit log by id
//	@Schemes
//	@Description	Get a single audit log by its id
//	@Tags			auditlogs
//	@Accept			application/json
//	@Produce		application/json
//	@Param			id						path	string	true	"id"
//	@Success		200						{object}	apicontracts.AuditLog
//	@Failure		403						{object}	rorerror.ErrorData
//	@Failure		400						{object}	rorerror.ErrorData
//	@Failure		401						{object}	rorerror.ErrorData
//	@Failure		500						{object}	rorerror.ErrorData
//	@Router			/v1/auditlogs/{id}		[get]
//	@Security		ApiKey || AccessToken
func GetById() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: global
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		id := c.Param("id")
		if id == "" || len(id) == 0 {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "invalid auditlog id")
			rerr.GinLogErrorAbort(c)
			return
		}

		auditlog, err := auditLogService.GetById(ctx, id)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get auditlog", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, auditlog)
	}
}

// GetMetadata returns metadata for audit logs.
//
//	@Summary	Get audit log metadata
//	@Schemes
//	@Description	Get metadata for audit logs
//	@Tags			auditlogs
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200							{object}	interface{}
//	@Failure		403							{object}	rorerror.ErrorData
//	@Failure		401							{object}	rorerror.ErrorData
//	@Failure		500							{object}	rorerror.ErrorData
//	@Router			/v1/auditlogs/metadata		[get]
//	@Security		ApiKey || AccessToken
func GetMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// Access check
		// Scope: ror
		// Subject: global
		// Access: read
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Read {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		metadata, err := auditLogService.GetMetadata(ctx)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "could not get metadata", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		c.JSON(http.StatusOK, metadata)
	}
}
