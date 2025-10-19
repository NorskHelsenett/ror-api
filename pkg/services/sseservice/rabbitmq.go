package sseservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/NorskHelsenett/ror/pkg/clients/rabbitmqclient"
	"github.com/NorskHelsenett/ror/pkg/handlers/rabbitmqhandler"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

const (
	SSERouteBroadcast       = "eventv2.broadcast"
	SSEventsExchange        = "ror.eventsv2"
	SSEventsQueueNamePrefix = "sse-events-v2"
)

func StartListeningRabbitMQ(rabbitMQConnection rabbitmqclient.RabbitMQConnection) {

	// //Create the queue
	SSEventsQueueName := fmt.Sprintf("%s-%s", SSEventsQueueNamePrefix, uuid.New().String())

	go func() {
		config := rabbitmqhandler.RabbitMQListnerConfig{
			Client:             rabbitMQConnection,
			QueueName:          SSEventsQueueName,
			Consumer:           "",
			AutoAck:            false,
			QueueAutoDelete:    true,
			Exclusive:          false,
			NoLocal:            false,
			NoWait:             false,
			Args:               nil,
			Exchange:           SSEventsExchange,
			ExcahngeKind:       "fanout",
			ExchangeAutoDelete: true,
			ExcahngeDurable:    true,
		}
		rabbithandler := rabbitmqhandler.New(config, ssemessagehandler{})
		_ = rabbitMQConnection.RegisterHandler(rabbithandler)

	}()
}

type ssemessagehandler struct {
}

func (amh ssemessagehandler) HandleMessage(ctx context.Context, message amqp091.Delivery) error {
	switch message.RoutingKey {
	case SSERouteBroadcast:
		err := HandleSSEEvent(ctx, message)
		if err != nil {
			rlog.Error("could not handle event", err)
			return err
		}
	default:
		rlog.Debugc(ctx, "could not handle message")
	}

	return nil
}

func HandleSSEEvent(ctx context.Context, message amqp091.Delivery) error {
	if message.Body == nil {
		return errors.New("message.body is nil")
	}

	var sseEvent SseEvent
	err := json.Unmarshal(message.Body, &sseEvent)
	if err != nil {
		return err
	}
	Server.Message <- EventMessage{
		Clients:  Server.Clients.GetBroadcast(),
		SseEvent: sseEvent,
	}
	return nil
}
