package ssehandler

import (
	"fmt"
	"io"
	"net/http"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/pkg/servers/sseserver"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	validate *validator.Validate
)

// @Summary	Server sent events
// @Schemes
// @Description	Listen to server sent events
// @Tags			events
// @Accept			text/event-stream
// @Produce		text/event-stream
// @Success		200					{string}	string	"ok"
// @Failure		403					{object}	rorerror.RorError
// @Failure		400					{object}	rorerror.RorError
// @Failure		401					{object}	rorerror.RorError
// @Failure		500					{object}	rorerror.RorError
// @Router			/v2/events/listen	[get]
// @Security		ApiKey || AccessToken
func HandleSSE() gin.HandlerFunc {
	return func(c *gin.Context) {
		v, ok := c.Get("sseClient")
		if !ok {
			return
		}
		client, ok := v.(*sseserver.EventClient)
		if !ok {
			return
		}
		c.Stream(func(w io.Writer) bool {
			if msg, ok := <-client.Connection; ok {
				fmt.Println("Sending message to client", msg)
				c.SSEvent(msg.Event, msg.Data)
				return true
			}
			return false
		})
	}
}

func Send() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		// Access check
		// Scope: ror
		// Subject: global
		// Access: create
		// TODO: check if this is the right way to do it
		accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		if !accessObject.Create {
			c.JSON(http.StatusForbidden, "403: No access")
			return
		}

		var input sseserver.SseEvent
		err := c.BindJSON(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Object is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		// err = validate.Struct(&input)
		// if err != nil {
		// 	rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields missing", err)
		// 	rerr.GinLogErrorAbort(c)
		// 	return
		// }

		fmt.Println("Sending message to clients")

		sseserver.Server.Message <- sseserver.EventMessage{
			Clients: sseserver.Server.Clients.GetBroadcast(),
			SseEvent: sseserver.SseEvent{
				Event: input.Event,
				Data:  input.Data,
			},
		}

		c.JSON(http.StatusOK, nil)
	}
}
