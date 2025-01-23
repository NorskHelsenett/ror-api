package sseserver

import identitymodels "github.com/NorskHelsenett/ror/pkg/models/identity"

type EventClientId string

type EventClientChan chan string

type EventClient struct {
	Id         EventClientId
	Connection EventClientChan
	Identity   identitymodels.Identity
}

type EventClients []*EventClient

func (e EventClients) Len() int {
	return len(e)
}

func (e EventClients) Get(id EventClientId) *EventClient {
	for _, client := range e {
		if client.Id == id {
			return client
		}
	}
	return nil
}

func (e *EventClients) Remove(id EventClientId) {
	for i, client := range *e {
		if client.Id == id {
			(*e)[i] = (*e)[len(*e)-1]
			(*e) = (*e)[:len(*e)-1]
			break
		}
	}
}

func (e *EventClients) Add(client *EventClient) {
	*e = append(*e, client)
}
