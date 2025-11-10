package sse

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"time"

	aclservice "github.com/NorskHelsenett/ror-api/internal/acl/services"
	"github.com/NorskHelsenett/ror-api/internal/models/ssemodels"
	"github.com/NorskHelsenett/ror-api/pkg/services/sseservice"

	"github.com/NorskHelsenett/ror/pkg/clients/rabbitmqclient"
	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

	aclmodels "github.com/NorskHelsenett/ror/pkg/models/aclmodels"

	"github.com/NorskHelsenett/ror/pkg/apicontracts"
	"github.com/NorskHelsenett/ror/pkg/helpers/rorerror/v2"

	"github.com/NorskHelsenett/ror/pkg/rlog"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

var (
	Server   *SSE
	validate *validator.Validate
)

func init() {
	validate = validator.New()
}

type SSEInterface interface {
	HandleSSE() gin.HandlerFunc
	SendMessage(topic string, payload string)
}

type SSE struct {
	SSEClients         []apicontracts.SSEClient
	RabbitMQConnection rabbitmqclient.RabbitMQConnection
	lock               sync.RWMutex
}

func Init(rabbitMQConnection rabbitmqclient.RabbitMQConnection) {
	Server = &SSE{
		lock:               sync.RWMutex{},
		SSEClients:         make([]apicontracts.SSEClient, 0),
		RabbitMQConnection: rabbitMQConnection,
	}
	KeepAlive()
	sseservice.StartEventServer(rabbitMQConnection)
}

func (sse *SSE) BroadcastMessage(payload ssemodels.SseMessage) {
	messageBytes, err := json.Marshal(payload)
	if err != nil {
		rlog.Error("could not marshal message", nil)
		return
	}
	message := fmt.Sprintf("data: %s\n\n", messageBytes)
	sse.lock.RLock()
	for _, client := range sse.SSEClients {
		if client.Connection != nil {
			select {
			case client.Connection <- message:
			default:
			}
		}
	}
	sse.lock.RUnlock()
}

func (sse *SSE) SendToUsersWithGroup(payload ssemodels.SseMessage, group string) {
	message, shouldReturn := prepMessage(payload)
	if shouldReturn {
		return
	}

	var clients []apicontracts.SSEClient
	for _, client := range sse.SSEClients {
		if client.Identity.IsUser() && slices.Contains(client.Identity.User.Groups, group) {
			clients = append(clients, client)
		}
	}

	sendMessage(sse, clients, message)
}

func (sse *SSE) SendToUsersWithGroups(payload ssemodels.SseMessage, groups []string) {
	message, shouldReturn := prepMessage(payload)
	if shouldReturn {
		return
	}

	var clients []apicontracts.SSEClient
	for _, client := range sse.SSEClients {
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

func sendMessage(sse *SSE, clients []apicontracts.SSEClient, message string) {
	sse.lock.RLock()
	for _, client := range clients {
		select {
		case client.Connection <- message:
		default:
		}
	}
	sse.lock.RUnlock()
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
// @Router			/v1/events/listen	[get]
// @Security		ApiKey || AccessToken
func (sse *SSE) HandleSSE() gin.HandlerFunc {
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
		sse.SSEClients = append(sse.SSEClients, client)
		SendWelcomeMessage(client)

		rlog.Debugc(ctx, "Listen to sse events", rlog.Any("total clients", len(sse.SSEClients)))

		defer func() {
			sse.lock.RLock()
			for i := 0; i < len(sse.SSEClients); i++ {
				cl := sse.SSEClients[i]
				if cl.Identity == client.Identity {
					sse.SSEClients = append(sse.SSEClients[:i], sse.SSEClients[i+1:]...)
					close(cl.Connection)
					break
				}
			}
			sse.lock.RUnlock()
			rlog.Debugc(ctx, "A client disconnected", rlog.Any("total clients", len(sse.SSEClients)))
		}()

		c.Stream(func(w io.Writer) bool {
			clientGone := c.Request.Context().Done()
			c.Stream(func(w io.Writer) bool {
				select {
				case <-clientGone:
					return false
				case message := <-client.Connection:
					//c.SSEvent("message", message)
					if message == "" {
						return false
					}
					_, err = w.Write([]byte(message))
					if err != nil {
						panic(err)
					}
					return true
				}
			})

			return true
		})
	}
}

func (sse *SSE) Send() gin.HandlerFunc {
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

		var input apicontracts.SSEMessage
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
		err = sse.RabbitMQConnection.SendMessage(ctx, message, messagebuscontracts.Event_Broadcast, nil)
		if err != nil {
			rlog.Errorc(ctx, "could not send sse broadcast event", err)
		}

		c.JSON(http.StatusOK, nil)
	}
}

func getClientFromRequest(identity identitymodels.Identity, connection chan string) (apicontracts.SSEClient, error) {
	client := apicontracts.SSEClient{
		Identity:   identity,
		Connection: connection,
	}

	return client, nil
}

func KeepAlive() {
	go func() {
		for {
			now := time.Now()
			payload := ssemodels.SseMessage{
				Event: ssemodels.SseType_Time,
				Data:  now,
			}
			Server.BroadcastMessage(payload)
			time.Sleep(time.Second * 15)
		}
	}()
}

func SendWelcomeMessage(client apicontracts.SSEClient) {
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
