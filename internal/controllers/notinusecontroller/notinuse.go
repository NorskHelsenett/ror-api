package notinusecontroller

import (
	"fmt"

	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/gin-gonic/gin"
)

func NotInUse() gin.HandlerFunc {
	return func(c *gin.Context) {
		rlog.Info(fmt.Sprintf("Hi %s you have reached a phone number not in use ... : %s%s\n", c.Request.Header["X-Forwarded-For"], c.Request.Host, c.Request.URL))
	}
}
