package healthginhandler

import (
	"net/http"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/rorginerror"
	"github.com/dotse/go-health"
	"github.com/gin-gonic/gin"
)

func GetGinHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := health.CheckNow(c.Request.Context())
		if err != nil {
			rerr := rorginerror.NewRorGinError(http.StatusInternalServerError, "Unable to run health checks", err)
			rerr.GinLogErrorAbort(c)
			return
		}
		if resp.Status == health.StatusFail {
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}

		bytes, _ := resp.Status.MarshalJSON()
		c.Data(http.StatusOK, "application/json", bytes)

	}
}
