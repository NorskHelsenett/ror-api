package sseservice

import (
	"sync"

	identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"
)

type SseEvent struct {
	Event string `json:"event"`
	Data  string `json:"data" validate:"required"`
}

type SSESubscribe struct {
	ClientId EventClientId `json:"clientId" validate:"required"`
	Topic    Subscription  `json:"topic" validate:"required"`
}

type Subscription string

type EventClientId string

type EventClientChan chan SseEvent

type EventClient struct {
	Id            EventClientId
	Connection    EventClientChan
	Identity      identitymodels.Identity
	Subscriptions []Subscription
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
	Server.Message <- EventMessage{
		Clients: []EventClientId{client.Id},
		SseEvent: SseEvent{
			Event: "connection.id",
			Data:  string(client.Id),
		},
	}
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

func (e *EventClient) Subscribe(topic Subscription) {
	e.Subscriptions = append(e.Subscriptions, topic)
}

func (e *EventClient) Unsubscribe(topic Subscription) {
	for i, t := range e.Subscriptions {
		if t == topic {
			e.Subscriptions = append(e.Subscriptions[:i], e.Subscriptions[i+1:]...)
			break
		}
	}
}
