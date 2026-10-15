package rabbitmq

import (
	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PreparePromotionOfferQueue prepara a fila que recebe as ofertas.
func PreparePromotionOfferQueue(channel *amqp.Channel) error {
	if err := DeclareExchange(
		channel,
		events.ExchangePromotions,
		ExchangeTypeTopic,
	); err != nil {
		return err
	}

	queue, err := DeclareQueue(channel, events.QueuePromotionOffers)
	if err != nil {
		return err
	}

	return BindQueue(
		channel,
		queue.Name,
		events.AllPromotions,
		events.ExchangePromotions,
	)
}
