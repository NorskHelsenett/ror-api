package resourcescontroller

import (
	"fmt"
	"net/http"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiservices/resourcesv2service"
	"github.com/NorskHelsenett/ror-api/internal/responses"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/NorskHelsenett/ror/pkg/rorresources"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
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
//	@Summary	Register  resource
//	@Schemes
//	@Description	Registers a  resource
//	@Tags			resources
//	@Accept			application/json
//	@Produce		application/json
//	@Param			rorresource	body		rorresources.ResourceSet	true	"ResourceUpdate"
//	@Success		200			{bool}		bool
//	@Failure		403			{string}	Forbidden
//	@Failure		401			{object}	rorerror.RorError
//	@Failure		500			{string}	Failure	message
//	@Router			/v2/resources [post]
//	@Security		ApiKey || AccessToken
func NewResource() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		var input rorresources.ResourceSet

		//validate the request body
		if err := c.BindJSON(&input); err != nil {
			rlog.Error("error binding json", err)
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": err.Error()}})
			return
		}
		//use the validator library to validate required fields
		if validationErr := validate.Struct(&input); validationErr != nil {
			c.JSON(http.StatusBadRequest, responses.Cluster{Status: http.StatusBadRequest, Message: "error", Data: map[string]interface{}{"data": validationErr.Error()}})
			return
		}
		rs := rorresources.NewResourceSetFromStruct(input)
		start := time.Now()

		returnChannel := make(chan rorresources.ResourceUpdateResults, len(rs.Resources))

		returnArray := rorresources.ResourceUpdateResults{}
		returnArray.Results = make(map[string]rorresources.ResourceUpdateResult, len(rs.Resources))

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
		rlog.Debug(fmt.Sprintf("%d resources changed in %s", len(rs.Resources), time.Since(start)))
		resourcesProcessed.Add(float64(len(rs.Resources)))
		resourcesRequests.Inc()

		c.JSON(http.StatusCreated, returnArray)
	}
}
