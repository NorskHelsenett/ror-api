// TODO: Describe package
package healthcontroller

import (
	"context"
	"net/http"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"

	"github.com/dotse/go-health"

	"github.com/gin-gonic/gin"
)

// TODO: Describe function
//
//	@BasePath	/
//	@Summary	Health status
//	@Schemes
//	@Description	Get health status for ROR-API
//	@Tags			health status
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200	{object}	health.Response
//	@Failure		500	{object}	Failure	message
//	@Router			/health [get]
func GetHealthStatus() gin.HandlerFunc {
	return func(c *gin.Context) {
		var healthstatus *health.Response
		var err error
		healthstatus, err = health.CheckHealth(context.Background())
		if err != nil {
			rerr := rorerror.NewRorError(500, "could not get health status")
			rerr.GinLogErrorAbort(c)
			return
		}
		c.JSON(http.StatusOK, healthstatus)
	}
}
