// TODO: Describe function
package metricscontroller

import (
	"net/http"

	metricsservice "github.com/NorskHelsenett/ror-api/internal/apiservices/metricsService"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

func init() {
	rlog.Debug("init cluster controller")
	validate = validator.New()
}

// TODO: Describe function
//
//	@Summary	Get metrics for workspaces
//	@Schemes
//	@Description	Get metrics for workspaces
//	@Tags			metrics
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200								{object}	apicontracts.PaginatedResult[apicontracts.Metric]
//	@Failure		403								{string}	Forbidden
//	@Failure		400								{object}	rorerror.ErrorData
//	@Failure		401								{object}	rorerror.ErrorData
//	@Failure		500								{string}	Failure	message
//	@Router			/v1/metrics/workspaces/filter	[post]
//	@Param			filter							body	apicontracts.Filter	true	"Filter"
//	@Security		ApiKey || AccessToken
func GetForWorkspaces() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		var filter apicontracts.Filter
		defer cancel()

		if err := c.BindJSON(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		for i := 0; i < len(filter.Sort); i++ {
			sort := filter.Sort[i]

			if err := validate.Struct(sort); err != nil {
				rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
				rerr.GinLogErrorAbort(c)
				return
			}
		}

		// importing apicontracts for swagger
		var _ apicontracts.PaginatedResult[apicontracts.Metric]

		metrics, err := metricsservice.GetForWorkspaces(ctx, &filter)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not get metrics for workspaces", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if metrics == nil {
			emptyResult := apicontracts.PaginatedResult[apicontracts.Metric]{
				Data:       make([]apicontracts.Metric, 0),
				DataCount:  0,
				TotalCount: 0,
				Offset:     0,
			}
			c.JSON(http.StatusOK, &emptyResult)
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}

// TODO: Describe function
//
//	@Summary	Get metrics for workspaces by datacenter name
//	@Schemes
//	@Description	Get metrics for workspaces by datacenter name
//	@Tags			metrics
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200														{object}	apicontracts.MetricList
//	@Failure		400														{object}	rorerror.ErrorData
//	@Failure		401														{object}	rorerror.ErrorData
//	@Failure		500														{string}	Failure	message
//	@Param			datacenterId											path		string	true	"datacenterId"
//	@Router			/v1/metrics/workspaces/datacenter/{datacenterId}/filter	[post]
//	@Param			filter													body	apicontracts.Filter	true	"Filter"
//	@Security		ApiKey || AccessToken
func GetForWorkspacesByDatacenterId() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		datacenterId := c.Param("datacenterId")
		var filter apicontracts.Filter
		defer cancel()

		if err := c.BindJSON(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "Missing parameter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		//use the validator library to validate required fields
		if err := validate.Struct(&filter); err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		for i := 0; i < len(filter.Sort); i++ {
			sort := filter.Sort[i]

			if err := validate.Struct(sort); err != nil {
				rerr := rorginerror.NewRorGinError(http.StatusBadRequest, "could not validate input", err)
				rerr.GinLogErrorAbort(c)
				return
			}
		}

		result, err := metricsservice.GetForWorkspacesByDatacenterId(ctx, &filter, datacenterId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusNotFound, "Could not get metris for workspaces by datacenter", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if result == nil {
			empty := apicontracts.PaginatedResult[apicontracts.Metric]{}
			c.JSON(http.StatusOK, empty)
			return
		}

		c.JSON(http.StatusOK, result)
	}
}

// TODO: Describe function
//
//	@Summary	Get metrics for workspace name
//	@Schemes
//	@Description	Get metrics for workspace name
//	@Tags			metrics
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200									{object}	apicontracts.MetricItem
//	@Failure		403									{string}	Forbidden
//	@Failure		400									{object}	rorerror.ErrorData
//	@Failure		401									{object}	rorerror.ErrorData
//	@Failure		500									{string}	Failure	message
//	@Router			/v1/metrics/workspace/{workspaceId}	[get]
//	@Param			workspaceId							path	string	true	"workspaceId"
//	@Security		ApiKey || AccessToken
func GetByWorkspaceId() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		workspaceId := c.Param("workspaceId")
		defer cancel()

		metrics, err := metricsservice.GetForWorkspaceId(ctx, workspaceId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not get metris for workspace", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if metrics == nil {
			empty := apicontracts.MetricItem{}
			c.JSON(http.StatusOK, empty)
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}
