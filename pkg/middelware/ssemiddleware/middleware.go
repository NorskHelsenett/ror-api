package ssemiddleware

import (
	"strings"

	"github.com/NorskHelsenett/ror/pkg/config/rorconfig"
	"github.com/gin-gonic/gin"
)

func SSEHeadersMiddlewareV2() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")
		c.Next()
	}
}

func SSEHeadersMiddlewareV1() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")

		c.Writer.Header().Set("Access-Control-Allow-Headers", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "OPTIONS, GET")
		requestOrigin := c.Request.Header.Get("Origin")
		allowOrigins := rorconfig.GetString(rorconfig.GIN_ALLOW_ORIGINS)
		origins := strings.Split(allowOrigins, ";")
		for _, origin := range origins {
			if strings.Contains(requestOrigin, origin) {
				c.Writer.Header().Set("Access-Control-Allow-Origin", requestOrigin)
			}
		}

		c.Next()
	}
}
