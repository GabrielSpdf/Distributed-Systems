package orders

import (
	"crypto/rsa"
	"fmt"
	"sync"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

type EventPublisher struct {
	channel    *amqp.Channel
	privateKey *rsa.PrivateKey
	mutex      sync.Mutex
}

func NewEventPublisher(
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
) *EventPublisher {
	return &EventPublisher{
		channel:    channel,
		privateKey: privateKey,
	}
}

func (publisher *EventPublisher) PublishCreated(order Order) error {
	if publisher.channel == nil {
		return fmt.Errorf("canal RabbitMQ não configurado")
	}

	event, err := BuildOrderCreatedEvent(order, publisher.privateKey)
	if err != nil {
		return fmt.Errorf("erro ao preparar pedido.criado: %w", err)
	}

	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()

	return rabbitmq.PublishEvent(
		publisher.channel,
		events.ExchangeEcommerce,
		events.OrderCreated,
		event,
	)
}
