package msentrega

import (
	"encoding/json"
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Runtime struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
	QueueName  string
}

func InitMSEntrega() (
	*Runtime,
	error,
) {
	connection, err := rabbitmq.Connect()
	if err != nil {
		return nil, err
	}
	log.Println("[SUCESSO] Conexão estabelecida com o RabbitMQ")

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Canal RabbitMQ aberto")

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Exchanges declaradas")

	deliveryQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueueEntrega,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila entrega declarada")

	for _, routingKey := range []string{
		events.PagamentoAprovado,
	} {
		if err := rabbitmq.BindQueue(
			channel,
			deliveryQueue.Name,
			routingKey,
			events.ExchangeEcommerce,
		); err != nil {
			channel.Close()
			connection.Close()
			return nil, err
		}
		log.Println("[SUCESSO] Fila entrega ligada à exchange Ecommerce e vinculada ao roteamento", routingKey)
	}

	runTime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  deliveryQueue.Name,
	}

	return runTime, nil
}

func HandleDeliveryEvent(
	envelope events.EventEnvelope,
	channel *amqp.Channel,
) error {
	var payload events.PaymentResultPayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf(
			"erro ao desserializar pagamento.aprovado: %w",
			err,
		)
	}

	switch envelope.EventType {
	case events.PagamentoAprovado:
		invoiceID := fmt.Sprintf("INV-%s", payload.OrderID)
		trackingCode := fmt.Sprintf("BR%sBR", payload.OrderID)

		err := PublishOrderShipped(
			channel,
			payload.OrderID,
			invoiceID,
			trackingCode,
		)

		if err != nil {
			return fmt.Errorf(
				"erro ao processar entrega: %w",
				err,
			)
		}

		log.Printf(
			"[SUCESSO] Entrega do pedido %s realizado com sucesso!",
			payload.OrderID,
		)

	default:
		return fmt.Errorf(
			"tipo de evento inesperado na Entrega: %s",
			envelope.EventType,
		)
	}

	return nil
}

func PublishOrderShipped(
	channel *amqp.Channel,
	orderId string,
	invoiceID string,
	trackingCode string,
) error {
	payload := events.OrderShippedPayload{
		OrderID:      orderId,
		InvoiceID:    invoiceID,
		TrackingCode: trackingCode,
	}

	envelope, err := misc.MountEnvelope(
		payload,
		events.PedidoEnviado,
		"ms-entrega",
		"signature",
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PedidoEnviado,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}
