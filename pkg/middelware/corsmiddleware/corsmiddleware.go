package corsmiddleware

import (
	"strings"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS returns a gin middleware for handling CORS
func CORS() gin.HandlerFunc {

	if rorconfig.GetBool(rorconfig.GIN_USE_CORS) {
		corsConfig := cors.DefaultConfig()
		//corsConfig.AllowCredentials = true

		origins := strings.Split(rorconfig.GetString(rorconfig.GIN_ALLOW_ORIGINS), ";")
		if len(origins) == 0 {
			origins = []string{"*"}
		}
		corsConfig.AllowOrigins = origins

		corsConfig.AddAllowHeaders("authorization")
		return cors.New(corsConfig)
	}

	return func(c *gin.Context) {
		c.Next()
	}
}
