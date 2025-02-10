package sseserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/NorskHelsenett/ror-api/internal/apiconnections"
	"github.com/NorskHelsenett/ror/pkg/handlers/rabbitmqhandler"
	"github.com/NorskHelsenett/ror/pkg/messagebuscontracts"
	"github.com/NorskHelsenett/ror/pkg/rlog"
	"github.com/google/uuid"
	"github.com/rabbitmq/amqp091-go"
)

const (
	SSERouteBroadcast       = "eventv2.broadcast"
	SSEventsExchange        = "ror.eventsv2"
	SSEventsQueueNamePrefix = "sse-events-v2"
)

func StartListeningRabbitMQ() {

	err := apiconnections.RabbitMQConnection.GetChannel().ExchangeDeclare(
		SSEventsExchange, // name
		"fanout",         // kind
		true,             // durable
		true,             // autoDelete -> delete when unused
		false,            // internal
		false,            // no-wait
		nil,              // arguments
	)
	if err != nil {
		rlog.Fatal("Could not declare excahnge", err)
	}

	err = apiconnections.RabbitMQConnection.GetChannel().ExchangeBind(
		SSEventsExchange,                //destination
		"eventv2.#",                     // key
		messagebuscontracts.ExchangeRor, // source
		false,                           // noWait
		nil,                             // arguments
	)
	if err != nil {
		panic(err)
	}

	// //Create the queue
	SSEventsQueueName := fmt.Sprintf("%s-%s", SSEventsQueueNamePrefix, uuid.New().String())
	// apiEventsqueue, err := apiconnections.RabbitMQConnection.GetChannel().QueueDeclare(
	// 	SSEventsQueueName, // name
	// 	false,             // durable
	// 	true,              // delete when unused
	// 	false,             // exclusive
	// 	false,             // no-wait
	// 	nil,               // arguments, non quorum queue
	// )
	// if err != nil {
	// 	rlog.Fatal("Could not declare queue", err)
	// }

	// err = apiconnections.RabbitMQConnection.GetChannel().QueueBind(
	// 	apiEventsqueue.Name, // queue name
	// 	"",                  // routing key
	// 	SSEventsExchange,    // exchange
	// 	false,
	// 	nil,
	// )
	// if err != nil {
	// 	rlog.Fatal("Could not bind queue to excahnge", err)
	// }

	go func() {
		config := rabbitmqhandler.RabbitMQListnerConfig{
			Client:          apiconnections.RabbitMQConnection,
			QueueName:       SSEventsQueueName,
			Consumer:        "",
			AutoAck:         false,
			QueueAutoDelete: true,
			Exclusive:       false,
			NoLocal:         false,
			NoWait:          false,
			Args:            nil,
			Exchange:        SSEventsExchange,
		}
		rabbithandler := rabbitmqhandler.New(config, ssemessagehandler{})
		_ = apiconnections.RabbitMQConnection.RegisterHandler(rabbithandler)

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
