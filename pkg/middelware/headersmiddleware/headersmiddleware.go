package headersmiddleware

import (
	"github.com/NorskHelsenett/ror/pkg/config/rorversion"
	"github.com/gin-gonic/gin"
)

func HeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("x-ror-version", rorversion.GetRorVersion().GetVersion())
		c.Header("x-ror-libver", rorversion.GetRorVersion().GetLibVer())
		c.Next()
	}
}
