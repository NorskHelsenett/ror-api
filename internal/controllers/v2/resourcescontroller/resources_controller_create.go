package resourcescontroller

import (
	"errors"
	"maps"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/helpers/responsehelper"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"

	"github.com/NorskHelsenett/ror/pkg/rorresources"
	"github.com/NorskHelsenett/ror/pkg/telemetry/rortracer"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
)

var (
	// resourcesProcessed is a Prometheus counter for the number of processed resources
	resourcesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "resources_processed_total",
		Help: "The total number of processed resources",
	})
	resourcesRequests = promauto.NewCounter(prometheus.CounterOpts{
		Name: "resources_requests_total",
		Help: "The total number of resource requests",
	})
)

// Register a new resource, the resource is in the payload.
// Parameter clusterid must match authorized clusterid
//
//	@Summary	Register resource
//	@Schemes
//	@Description	Registers a resource
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			rorresource	body		rorresources.ResourceSet	true	"ResourceUpdate"
//	@Success		201			{object}	rorresources.ResourceUpdateResults
//	@Failure		403			{string}	Forbidden
//	@Failure		401			{object}	rorerror.ErrorData
//	@Failure		500			{string}	Failure	message
//	@Router			/v2/resources [post]
//	@Security		ApiKey || AccessToken
func NewResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		ctx, span := rortracer.StartSpan(ctx, "v2.resourcescontroller.NewResource")
		defer span.End()
		var input rorresources.ResourceSet

		//validate the request body
		if err := c.BindJSON(&input); err != nil {
			_ = rortracer.SpanError(span, err, "failed to bind JSON")
			rlog.Error("error binding json", err)
			responsehelper.ErrorResponse(c, http.StatusBadRequest, err)
			return
		}
		//use the validator library to validate required fields
		if validationErr := validate.Struct(&input); validationErr != nil {
			_ = rortracer.SpanError(span, validationErr, "validation failed")
			rlog.Error("validation failed", validationErr)
			responsehelper.ErrorResponse(c, http.StatusBadRequest, validationErr)
			return
		}
		span.AddEvent("request validated")

		rs, err := rorresources.NewResourceSetFromStruct(input)
		switch {
		case err == nil:
			// No error, continue processing
		case errors.Is(err, rorresources.ErrResourceSetEmpty):
			_ = rortracer.SpanError(span, err, "resource set is empty")
			rlog.Error("resource set is empty", err)
			responsehelper.ErrorResponse(c, http.StatusBadRequest, err)
			return

		case errors.Is(err, rorresources.ErrUnknownResourceKind):
			_ = rortracer.SpanError(span, err, "unknown resource kind")
			rlog.Error("unknown resource kind", err)
			responsehelper.ErrorResponse(c, http.StatusBadRequest, err)
			return
		default:
			_ = rortracer.SpanError(span, err, "failed to create resource set from struct")
			rlog.Error("error creating resource set from struct", err)
			responsehelper.ErrorResponse(c, http.StatusInternalServerError, err)
			return
		}

		span.SetAttributes(attribute.Int("resources.count", len(rs.Resources)))

		returnChannel := make(chan rorresources.ResourceUpdateResults, len(rs.Resources))

		returnArray := rorresources.ResourceUpdateResults{}
		returnArray.Results = make(map[string]rorresources.ResourceUpdateResult, len(rs.Resources))

		span.AddEvent("processing started")
		for _, resource := range rs.Resources {
			go func(res *rorresources.Resource, returnChan chan rorresources.ResourceUpdateResults) {
				returnChannel <- resourcesv2service.HandleResourceUpdate(ctx, res)
			}(resource, returnChannel)
		}

		for i := 0; i < len(rs.Resources); i++ {
			result := <-returnChannel
			maps.Copy(returnArray.Results, result.Results)
		}
		span.AddEvent("processing complete")

		resourcesProcessed.Add(float64(len(rs.Resources)))
		resourcesRequests.Inc()

		rortracer.SpanOk(span)
		c.JSON(http.StatusCreated, returnArray)
	}
}
