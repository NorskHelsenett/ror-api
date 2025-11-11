package ssehandler

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/pkg/services/sseservice"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/gin-gonic/gin"
)

// @Summary	Server sent events
// @Schemes
// @Description	Listen to server sent events
// @Tags			events
// @Accept			text/event-stream
// @Produce		text/event-stream
// @Success		200					{string}	string	"ok"
// @Failure		403					{object}	rorerror.ErrorData
// @Failure		400					{object}	rorerror.ErrorData
// @Failure		401					{object}	rorerror.ErrorData
// @Failure		500					{object}	rorerror.ErrorData
// @Router			/v2/events/listen	[get]
// @Security		ApiKey || AccessToken
func HandleSSE() gin.HandlerFunc {
	return func(c *gin.Context) {
		stopChan := make(chan bool)
		var writeLock sync.Mutex

		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		identity := rorcontext.GetIdentityFromRorContext(ctx)
		client := &sseservice.EventClient{
			Id:         sseservice.NewEventClientId(),
			Identity:   identity,
			Connection: make(sseservice.EventClientChan),
		}
		sseservice.Server.NewClients <- client
		// Send new connection to event server

		defer func() {
			stopChan <- true
		}()
		go func() {
			for {
				select {
				case <-stopChan:
					go func() {
						for range client.Connection {
						}
					}()
					// Send closed connection to event server
					sseservice.Server.ClosedClients <- client.Id
					cancel()
					return
				default:
					time.Sleep(time.Second * 1)
					writeLock.Lock()
					_, _ = c.Writer.Write([]byte(":keepalive\n"))
					c.Writer.Flush()
					writeLock.Unlock()
				}
			}
		}()

		c.Stream(func(w io.Writer) bool {
			select {
			case msg, ok := <-client.Connection:
				if ok {
					writeLock.Lock()
					c.SSEvent(msg.Event, msg.Data)
					writeLock.Unlock()
					return true
				}
				return false
			case <-c.Request.Context().Done():
				stopChan <- true
				return false
			}
		})
	}
}

func Send() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		// // // Access check
		// // // Scope: ror
		// // // Subject: global
		// // // Access: create
		// // // TODO: check if this is the right way to do it
		// accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		// accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		// if !accessObject.Create {
		// 	c.JSON(http.StatusForbidden, "403: No access")
		// 	return
		// }

		var input sseservice.SseEvent
		err := c.BindJSON(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Object is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		err = apiconnections.RabbitMQConnection.SendMessage(ctx, input, sseservice.SSERouteBroadcast, nil)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "could not send sse broadcast event", err)
			rerr.GinLogErrorAbort(c)
		}
		c.JSON(http.StatusOK, nil)
	}
}

func Subscribe() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		// // // Access check
		// // // Scope: ror
		// // // Subject: global
		// // // Access: create
		// // // TODO: check if this is the right way to do it
		// accessQuery := aclmodels.NewAclV2QueryAccessScopeSubject(aclmodels.Acl2ScopeRor, aclmodels.Acl2RorSubjectGlobal)
		// accessObject := aclservice.CheckAccessByContextAclQuery(ctx, accessQuery)
		// if !accessObject.Create {
		// 	c.JSON(http.StatusForbidden, "403: No access")
		// 	return
		// }

		var input sseservice.SSESubscribe
		err := c.BindJSON(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Object is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		err = apiconnections.RabbitMQConnection.SendMessage(ctx, input, sseservice.SSERouteBroadcast, nil)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusInternalServerError, "could not send sse broadcast event", err)
			rerr.GinLogErrorAbort(c)
		}
		c.JSON(http.StatusOK, nil)
	}
}
