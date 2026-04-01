package infocontroller

import (
	"net/http"

	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	"github.com/gin-gonic/gin"
)

// TODO: Describe
//
//	@Summary	Get version
//	@Schemes
//	@Description	Get version
//	@Tags			info
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200					{object}	rorversion.RorVersion
//	@Failure		500					{object}	map[string]interface{}
//	@Router			/v1/info/version	[get]
func GetVersion() gin.HandlerFunc {
	return func(c *gin.Context) {
		var _ rorversion.RorVersion
		c.JSON(http.StatusOK, rorversion.GetRorVersion())
	}
}
