package sseserver

import (
	"sync"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
)

type SseEvent struct {
	Event string `json:"event"`
	Data  string `json:"data" validate:"required"`
}

type EventClientId string

type EventClientChan chan SseEvent

type EventClient struct {
	Id         EventClientId
	Connection EventClientChan
	Identity   identitymodels.Identity
}

type EventClients struct {
	clients []*EventClient
	lock    sync.RWMutex
}

func NewEventClients() EventClients {
	return EventClients{
		clients: make([]*EventClient, 0),
	}
}

func (e *EventClients) Len() int {
	e.lock.RLock()
	defer e.lock.RUnlock()
	return len(e.clients)
}

func (e *EventClients) Get(id EventClientId) *EventClient {
	e.lock.RLock()
	defer e.lock.RUnlock()
	for _, client := range e.clients {
		if client.Id == id {
			return client
		}
	}
	return nil
}

func (e *EventClients) Remove(id EventClientId) {
	e.lock.Lock()
	defer e.lock.Unlock()
	for i, client := range e.clients {
		if client.Id == id {
			e.clients[i] = e.clients[len(e.clients)-1]
			e.clients = e.clients[:len(e.clients)-1]
			break
		}
	}
}

func (e *EventClients) Add(client *EventClient) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.clients = append(e.clients, client)
}

func (e *EventClients) GetBroadcast() []EventClientId {
	e.lock.RLock()
	defer e.lock.RUnlock()
	var clients []EventClientId
	for _, client := range e.clients {
		clients = append(clients, client.Id)
	}
	return clients
}
