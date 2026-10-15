package rabbitmq

import (
	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PreparePromotionInterestQueue pode ser chamada pelo Gateway e pelo consumidor.
func PreparePromotionInterestQueue(channel *amqp.Channel) error {
	if err := DeclareExchange(
		channel,
		events.ExchangeEcommerce,
		ExchangeTypeDirect,
	); err != nil {
		return err
	}

	queue, err := DeclareQueue(channel, events.QueuePromotionInterests)
	if err != nil {
		return err
	}

	return BindQueue(
		channel,
		queue.Name,
		events.PromotionInterest,
		events.ExchangeEcommerce,
	)
}
