package sseserver

import (
	"log"

	"github.com/NorskHelsenett/ror/pkg/context/gincontext"
	"github.com/NorskHelsenett/ror/pkg/context/rorcontext"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var Server *EventServer

type EventServer struct {
	// Events are pushed to this channel by the main events-gathering routine
	Message chan EventMessage

	// New client connections
	NewClients chan *EventClient

	// Closed client connections
	ClosedClients chan EventClientId

	// Total client connections
	Clients EventClients
}

type EventMessage struct {
	Clients []EventClientId
	Message string
}

func StartEventServer() {
	Server = &EventServer{
		Message:       make(chan EventMessage),
		NewClients:    make(chan *EventClient),
		ClosedClients: make(chan EventClientId),
		Clients:       make(EventClients, 0),
	}

	go Server.listen()

}

// It Listens all incoming requests from clients.
// Handles addition and removal of clients and broadcast messages to clients.
func (es *EventServer) listen() {
	for {
		select {
		// Add new available client
		case client := <-es.NewClients:
			es.Clients.Add(client)
			log.Printf("Client added. %d registered clients", es.Clients.Len())

		// Remove closed client
		case client := <-es.ClosedClients:

			close(es.Clients.Get(client).Connection)
			es.Clients.Remove(client)
			log.Printf("Removed client. %d registered clients", es.Clients.Len())

		// Broadcast message to client
		case eventMsg := <-es.Message:
			for _, clientid := range eventMsg.Clients {
				es.Clients.Get(clientid).Connection <- eventMsg.Message
			}
		}
	}
}

func (stream *EventServer) ServeSSE() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Initialize client channel
		ctx, cancel := gincontext.GetRorContextFromGinContext(c)
		defer cancel()
		identity := rorcontext.GetIdentityFromRorContext(ctx)
		client := &EventClient{
			Id:         newEventId(),
			Identity:   identity,
			Connection: make(EventClientChan),
		}

		// Send new connection to event server
		stream.NewClients <- client

		defer func() {
			// Drain client channel so that it does not block. Server may keep sending messages to this channel
			go func() {
				for range <-client.Connection {
				}
			}()
			// Send closed connection to event server
			stream.ClosedClients <- client.Id
		}()

		c.Set("sseClient", client)

		c.Next()
	}
}

func newEventId() EventClientId {
	id, _ := uuid.NewUUID()
	return EventClientId(id.String())
}
