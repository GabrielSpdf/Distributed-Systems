package msentrega

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Runtime struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
	QueueName  string
	PrivateKey *rsa.PrivateKey
	PublicKeys cryptography.PublicKeyRegistry
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

	privateKey, err := cryptography.LoadPrivateKey("keys/ms-entrega/private.pem")
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, fmt.Errorf(
			"erro ao carregar chave privada do Entrega: %w",
			err,
		)
	}

	publicKeys, err := cryptography.LoadPublicKeyRegistry(
		map[string]string{
			events.ProducerPagamento: "keys/ms-entrega/public_keys/ms-pagamento.pem",
		},
	)
	if err != nil {
		channel.Close()
		connection.Close()

		return nil, fmt.Errorf(
			"erro ao carregar chave pública da Entrega: %w",
			err,
		)
	}

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

	runtime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  deliveryQueue.Name,
		PrivateKey: privateKey,
		PublicKeys: publicKeys,
	}

	return runtime, nil
}

func HandleDeliveryEvent(
	envelope events.EventEnvelope,
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
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
			privateKey,
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
	privateKey *rsa.PrivateKey,
	orderID string,
	invoiceID string,
	trackingCode string,
) error {
	payload := events.OrderShippedPayload{
		OrderID:      orderID,
		InvoiceID:    invoiceID,
		TrackingCode: trackingCode,
	}

	envelope, err := misc.MountSignedEnvelope(
		payload,
		events.PedidoEnviado,
		events.ProducerEntrega,
		privateKey,
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
