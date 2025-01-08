package infocontroller

import (
	"encoding/json"
	"net/http"

	"github.com/NorskHelsenett/ror-api/internal/apiconfig"

	"github.com/gin-gonic/gin"
)

type Version struct {
	Version string `json:"version"`
}

// TODO: Describe
//
//	@Summary	Get version
//	@Schemes
//	@Description	Get version
//	@Tags			info
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200					{object}	map[string]interface{}
//	@Failure		403					{object}	map[string]interface{}
//	@Failure		401					{object}	map[string]interface{}
//	@Failure		500					{object}	map[string]interface{}
//	@Router			/v1/info/version	[get]
func GetVersion() gin.HandlerFunc {
	return func(c *gin.Context) {
		res := apiconfig.RorVersion
		output, err := json.Marshal(res)
		if err != nil {
			c.JSON(http.StatusInternalServerError, "500: Could not marshal json")
			return
		}
		c.Data(http.StatusOK, "application/json", output)
	}
}
