package sseserver

import (
	"github.com/NorskHelsenett/ror/pkg/rlog"
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
	SseEvent
}

func StartEventServer() {
	Server = &EventServer{
		Message:       make(chan EventMessage),
		NewClients:    make(chan *EventClient),
		ClosedClients: make(chan EventClientId),
		Clients:       NewEventClients(),
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
			rlog.Infof("Added sse client. %d registered clients", es.Clients.Len())

		// Remove closed client
		case client := <-es.ClosedClients:

			close(es.Clients.Get(client).Connection)
			es.Clients.Remove(client)
			rlog.Infof("Removed sse client. %d registered clients", es.Clients.Len())

		// Broadcast message to client
		case eventMsg := <-es.Message:
			if len(eventMsg.Clients) > 0 {
				for _, clientid := range eventMsg.Clients {
					es.Clients.Get(clientid).Connection <- SseEvent{Event: eventMsg.Event, Data: eventMsg.Data}
				}
			}
		}
	}
}

func NewEventClientId() EventClientId {
	id, _ := uuid.NewUUID()
	return EventClientId(id.String())
}
