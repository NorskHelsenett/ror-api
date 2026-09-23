package auditmiddleware

import (
	"github.com/NorskHelsenett/ror-api/internal/auditlog"
	"github.com/NorskHelsenett/ror-api/internal/models"

	"github.com/NorskHelsenett/ror-api/pkg/helpers/gincontext"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/gin-gonic/gin"
)

func AuditLogMiddleware(msg string, category models.AuditCategory, action models.AuditAction) gin.HandlerFunc {
	return func(c *gin.Context) {
		actor, err := gincontext.GetActorFromGinContext(c)
		ctx := c.Request.Context()
		if err != nil {
			rlog.Errorc(ctx, "unable to get actor from auditlog middleware", err)
		}
		c.Next()
		if c.Writer.Status() != 200 {
			return
		}
		newObject, _ := c.Get("newObject")
		oldObject, _ := c.Get("oldObject")
		_, err = auditlog.Create(ctx, msg, category, action, actor, newObject, oldObject)
		if err != nil {
			rlog.Errorc(ctx, "could not create auditlog", err)
		}
	}
}
