// TODO: Describe function
package metricscontroller

import (
	"net/http"

	metricsservice "github.com/NorskHelsenett/ror-api/internal/apiservices/metricsService"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/NorskHelsenett/ror/pkg/context/gincontext"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"

	"github.com/gin-gonic/gin"
)

// TODO: Describe function
//
//	@Summary	Get metrics for datacenters
//	@Schemes
//	@Description	Get metrics for datacenters
//	@Tags			metrics
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200						{object}	apicontracts.MetricList
//	@Failure		403						{string}	Forbidden
//	@Failure		401						{object}	rorerror.ErrorData
//	@Failure		500						{string}	Failure	message
//	@Router			/v1/metrics/datacenters	[get]
//	@Security		ApiKey || AccessToken
func GetForDatacenters() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		// importing apicontracts for swagger
		var _ apicontracts.MetricList
		metrics, err := metricsservice.GetForDatacenters(ctx)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not get metrics", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		if metrics == nil {
			empty := apicontracts.MetricList{}
			c.JSON(http.StatusOK, empty)
			return
		}

		c.JSON(http.StatusOK, metrics)
	}
}

// TODO: Describe function
//
//	@Summary	Get metrics for datacenter name
//	@Schemes
//	@Description	Get metrics for datacenter name
//	@Tags			metrics
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200										{object}	apicontracts.MetricItem
//	@Failure		403										{string}	Forbidden
//	@Failure		401										{object}	rorerror.ErrorData
//	@Failure		500										{string}	Failure	message
//	@Router			/v1/metrics/datacenter/{datacenterName}	[get]
//	@Param			datacenterName							path	string	true	"datacenterName"
//	@Security		ApiKey || AccessToken
func GetByDatacenterId() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		datacenterId := c.Param("datacenterId")
		defer cancel()

		// importing apicontracts for swagger
		var _ apicontracts.MetricItem

		metrics, err := metricsservice.GetForDatacenterId(ctx, datacenterId)
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Could not get metrics", err)
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
