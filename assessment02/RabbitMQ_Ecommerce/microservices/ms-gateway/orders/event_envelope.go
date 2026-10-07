package orders

import (
	"crypto/rsa"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
)

// BuildOrderCreatedEvent monta e assina o evento de criação do pedido.
func BuildOrderCreatedEvent(
	order Order,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	payload := BuildOrderCreatedPayload(order)

	return misc.BuildSignedEnvelope(
		payload,
		events.OrderCreated,
		events.ProducerPrincipal,
		privateKey,
	)
}
