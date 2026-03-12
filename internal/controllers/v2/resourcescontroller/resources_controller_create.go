package resourcescontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/models/responses"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/NorskHelsenett/ror/pkg/rorresources"

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
		ctx, span := otel.GetTracerProvider().Tracer(rorconfig.GetString(rorconfig.TRACER_ID)).Start(ctx, "v2.resourcescontroller.NewResource")
		defer span.End()
		var input rorresources.ResourceSet

		//validate the request body
		if err := c.BindJSON(&input); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "failed to bind JSON")
			rlog.Error("error binding json", err)
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": err.Error()}})
			return
		}
		//use the validator library to validate required fields
		if validationErr := validate.Struct(&input); validationErr != nil {
			span.RecordError(validationErr)
			span.SetStatus(codes.Error, "validation failed")
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": validationErr.Error()}})
			return
		}
		span.AddEvent("request validated")

		rs := rorresources.NewResourceSetFromStruct(input)
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
			for key, result := range result.Results {
				returnArray.Results[key] = result
			}
		}
		span.AddEvent("processing complete")

		resourcesProcessed.Add(float64(len(rs.Resources)))
		resourcesRequests.Inc()

		span.SetStatus(codes.Ok, "")
		c.JSON(http.StatusCreated, returnArray)
	}
}
