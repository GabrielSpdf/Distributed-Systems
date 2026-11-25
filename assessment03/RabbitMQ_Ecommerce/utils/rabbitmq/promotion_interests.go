package rabbitmq

import (
	amqp "github.com/rabbitmq/amqp091-go"
)

// PreparePromotionInterestQueue pode ser chamada pelo Gateway e pelo consumidor.
func PreparePromotionInterestQueue(channel *amqp.Channel) error {
	return PrepareEcommerceTopology(channel)
}
