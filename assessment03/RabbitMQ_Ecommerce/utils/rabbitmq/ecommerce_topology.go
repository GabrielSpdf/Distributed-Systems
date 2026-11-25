package rabbitmq

import (
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

type ecommerceBinding struct {
	queue      string
	routingKey string
}

// A lista é comum aos produtores e consumidores do fluxo web. Cada serviço
// recebe sua própria cópia; Gateway não compete com Pagamento ou Entrega.
func ecommerceBindings() []ecommerceBinding {
	return []ecommerceBinding{
		{events.QueueStock, events.OrderCreated},
		{events.QueueStock, events.OrderDeleted},
		{events.QueuePayment, events.OrderStockConfirmed},
		{events.QueueDelivery, events.PaymentApproved},
		{events.QueueGatewayStatus, events.OrderStockConfirmed},
		{events.QueueGatewayStatus, events.StockUnavailable},
		{events.QueueGatewayStatus, events.PaymentCheckoutAvailable},
		{events.QueueGatewayStatus, events.PaymentApproved},
		{events.QueueGatewayStatus, events.PaymentRefused},
		{events.QueueGatewayStatus, events.OrderShipped},
		{events.QueuePromotionInterests, events.PromotionInterest},
	}
}

// PrepareEcommerceTopology prepara todas as filas e bindings conhecidos antes
// de publicar. Confirm + mandatory sozinho só prova que havia algum destino.
// Não exige consumidores online: filas duráveis aguardam o início dos serviços.
func PrepareEcommerceTopology(channel *amqp.Channel) error {
	return prepareEcommerceTopology(channel, events.ExchangeEcommerce, "")
}

// PrepareEcommerceTopologyForEvent é usada também nas novas conexões de
// publicação. Rotas auxiliares de teste não criam recursos de negócio.
func PrepareEcommerceTopologyForEvent(channel *amqp.Channel, eventType string) error {
	for _, binding := range ecommerceBindings() {
		if binding.routingKey == eventType {
			return PrepareEcommerceTopology(channel)
		}
	}
	return nil
}

// Os nomes parametrizados permitem testar a mesma preparação usando apenas
// exchange e filas exclusivas, sem alterar a topologia da demonstração.
func prepareEcommerceTopology(channel *amqp.Channel, exchangeName, queuePrefix string) error {
	if channel == nil {
		return fmt.Errorf("canal RabbitMQ não configurado para preparar topologia")
	}
	if err := DeclareExchange(channel, exchangeName, ExchangeTypeDirect); err != nil {
		return err
	}
	declared := make(map[string]bool)
	for _, binding := range ecommerceBindings() {
		queue := queuePrefix + binding.queue
		if !declared[queue] {
			if _, err := DeclareQueue(channel, queue); err != nil {
				return err
			}
			declared[queue] = true
		}
		if err := BindQueue(channel, queue, binding.routingKey, exchangeName); err != nil {
			return err
		}
	}
	return nil
}
