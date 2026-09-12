package mspagamento

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"

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

func InitMSPagamento() (
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

	paymentQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueuePagamento,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila pagamento declarada")

	for _, routingKey := range []string{
		events.PedidoEstoqueOk,
	} {
		if err := rabbitmq.BindQueue(
			channel,
			paymentQueue.Name,
			routingKey,
			events.ExchangeEcommerce,
		); err != nil {
			channel.Close()
			connection.Close()
			return nil, err
		}
		log.Println("[SUCESSO] Fila pagamento ligada à exchange Ecommerce e vinculada ao roteamento", routingKey)
	}

	runTime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  paymentQueue.Name,
	}

	return runTime, nil
}

func DecideApproval() bool {
	/*
		True = Pagamento aprovado (85%)
		False = Pagamento recusado (15%)
	*/
	return rand.Float64() > 0.15
}

func HandlePaymentEvent(
	envelope events.EventEnvelope,
	channel *amqp.Channel,
) error {
	var payload events.OrderReferencePayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf(
			"erro ao desserializar pedido.estoque_ok: %w",
			err,
		)
	}

	switch envelope.EventType {
	case events.PedidoEstoqueOk:
		if DecideApproval() {
			err := PublishPaymentOk(
				channel,
				payload.OrderID,
			)

			if err != nil {
				return fmt.Errorf(
					"erro ao processar pagamento: %w",
					err,
				)
			}

			log.Printf(
				"[SUCESSO] Pagamento do pedido %s realizado com sucesso!",
				payload.OrderID,
			)
		} else {
			if err := PublishPaymentNok(
				channel,
				payload.OrderID,
				"pagamento recusado pela operadora",
			); err != nil {
				return fmt.Errorf(
					"erro ao processar pagamento: %w",
					err,
				)
			}

			log.Printf(
				"[ERRO] Pagamento do pedido %s recusado...",
				payload.OrderID,
			)
		}

	default:
		return fmt.Errorf(
			"tipo de evento inesperado no Pagamento: %s",
			envelope.EventType,
		)
	}

	return nil
}

func PublishPaymentOk(
	channel *amqp.Channel,
	orderId string,
) error {
	payload := events.PaymentResultPayload{
		OrderID: orderId,
	}

	envelope, err := misc.MountEnvelope(
		payload,
		events.PagamentoAprovado,
		"ms-pagamento",
		"signature",
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PagamentoAprovado,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}

func PublishPaymentNok(
	channel *amqp.Channel,
	orderId string,
	reason string,
) error {
	payload := events.PaymentResultPayload{
		OrderID: orderId,
		Reason:  reason,
	}

	envelope, err := misc.MountEnvelope(
		payload,
		events.PagamentoRecusado,
		"ms-pagamento",
		"signature",
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PagamentoRecusado,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}
