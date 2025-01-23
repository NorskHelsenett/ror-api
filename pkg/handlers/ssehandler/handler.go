package ssehandler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror-api/internal/models/ssemodels"

	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	Server   *SSEHandler
	validate *validator.Validate
)

type SSEMessage struct {
	Event string `json:"event" validate:"required,min=1,ne=' '"`
	Data  any    `json:"data" validate:"required"`
}

type SSEClient struct {
	Identity   identitymodels.Identity `json:"identity"`
	Connection chan string             `json:"connection"`
}

func init() {
	validate = validator.New()
}

type SSEInterface interface {
	HandleSSE() gin.HandlerFunc
	SendMessage(topic string, payload string)
}

type SSEHandler struct {
	Clients []SSEClient
	lock    sync.RWMutex
}

func Init() {
	Server = NewSSEHandler()
}

func NewSSEHandler() *SSEHandler {
	return &SSEHandler{
		lock:    sync.RWMutex{},
		Clients: make([]SSEClient, 0),
	}
}

func (sse *SSEHandler) BroadcastMessage(payload ssemodels.SseMessage) {
	messageBytes, err := json.Marshal(payload)
	if err != nil {
		rlog.Error("could not marshal message", nil)
		return
	}
	message := fmt.Sprintf("data: %s\n\n", messageBytes)
	sse.lock.RLock()
	defer sse.lock.RUnlock()
	for _, client := range sse.Clients {
		if client.Connection != nil {
			select {
			case client.Connection <- message:
			default:
			}
		}
	}
}

func (sse *SSEHandler) SendToUsersWithGroup(payload ssemodels.SseMessage, group string) {
	message, shouldReturn := prepMessage(payload)
	if shouldReturn {
		return
	}

	var clients []SSEClient
	for _, client := range sse.Clients {
		if client.Identity.IsUser() && slices.Contains(client.Identity.User.Groups, group) {
			clients = append(clients, client)
		}
	}

	sendMessage(sse, clients, message)
}

func (sse *SSEHandler) SendToUsersWithGroups(payload ssemodels.SseMessage, groups []string) {
	message, shouldReturn := prepMessage(payload)
	if shouldReturn {
		return
	}

	var clients []SSEClient
	for _, client := range sse.Clients {
		if client.Identity.IsUser() {
			for _, group := range groups {
				if slices.Contains(client.Identity.User.Groups, group) {
					clients = append(clients, client)
					break
				}
			}
		}
	}

	sendMessage(sse, clients, message)
}

func prepMessage(payload ssemodels.SseMessage) (string, bool) {
	messageBytes, err := json.Marshal(payload)
	if err != nil {
		rlog.Error("could not marshal message", nil)
		return "", true
	}
	message := fmt.Sprintf("data: %s\n\n", messageBytes)
	return message, false
}

func sendMessage(sse *SSEHandler, clients []SSEClient, message string) {
	sse.lock.RLock()
	defer sse.lock.RUnlock()
	for _, client := range clients {
		select {
		case client.Connection <- message:
		default:
		}
	}
}

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
func (sse *SSEHandler) HandleSSE() gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()

		connection := make(chan string)
		identity := rorcontext.GetIdentityFromRorContext(ctx)
		client, err := getClientFromRequest(identity, connection)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Could not get client from request", err)
			rerr.GinLogErrorAbort(c)
			return
		}
		sse.lock.Lock()
		defer sse.lock.Unlock()
		sse.Clients = append(sse.Clients, client)
		sse.lock.Unlock()
		SendWelcomeMessage(client)

		rlog.Debugc(ctx, "Listen to sse events", rlog.Any("total clients", len(sse.Clients)))

		defer func() {
			sse.lock.RLock()
			defer sse.lock.RUnlock()
			for i := 0; i < len(sse.Clients); i++ {
				cl := sse.Clients[i]
				if cl.Identity == client.Identity {
					sse.Clients = append(sse.Clients[:i], sse.Clients[i+1:]...)
					close(cl.Connection)
					break
				}
			}
			sse.lock.RUnlock()
			rlog.Debugc(ctx, "A client disconnected", rlog.Any("total clients", len(sse.Clients)))
		}()

		c.Stream(func(w io.Writer) bool {
			clientGone := c.Request.Context().Done()
			//			c.Stream(func(w io.Writer) bool {
			select {
			case <-clientGone:
				return false
			case message := <-client.Connection:
				//c.SSEvent("message", message)
				if message == "" {
					return true
				}
				_, err = w.Write([]byte(message))
				if err != nil {
					panic(err)
				}
				return true
			}
			//			})

			//	return true
		})
	}
}

func (sse *SSEHandler) Send() gin.HandlerFunc {
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

		var input SSEMessage
		err := c.BindJSON(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Object is not valid", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		err = validate.Struct(&input)
		if err != nil {
			rerr := rorerror.NewRorError(http.StatusBadRequest, "Required fields missing", err)
			rerr.GinLogErrorAbort(c)
			return
		}

		message := ssemodels.SseMessage{Event: ssemodels.SseType(input.Event), Data: input.Data}
		err = apiconnections.RabbitMQConnection.SendMessage(ctx, message, messagebuscontracts.Event_Broadcast, nil)
		if err != nil {
			rlog.Errorc(ctx, "could not send sse broadcast event", err)
		}

		c.JSON(http.StatusOK, nil)
	}
}

func getClientFromRequest(identity identitymodels.Identity, connection chan string) (SSEClient, error) {
	client := SSEClient{
		Identity:   identity,
		Connection: connection,
	}

	return client, nil
}

func (sse *SSEHandler) KeepAlive() {
	go func() {
		for {
			now := time.Now()
			payload := ssemodels.SseMessage{
				Event: ssemodels.SseType_Time,
				Data:  now,
			}
			sse.BroadcastMessage(payload)
			time.Sleep(time.Second * 15)
		}
	}()
}

func SendWelcomeMessage(client SSEClient) {
	go func() {
		payload := map[string]interface{}{
			"message": "Welcome to ROR",
		}
		jsonData, err := json.Marshal(payload)
		if err != nil {
			return
		}
		client.Connection <- fmt.Sprintf("data: %s\n\n", jsonData)
	}()
}
