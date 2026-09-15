package mspagamento

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"

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

func InitializePaymentService() (
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

	privateKey, err := cryptography.LoadPrivateKey("keys/ms-pagamento/private.pem")
	if err != nil {
		channel.Close()
		connection.Close()

		return nil, fmt.Errorf(
			"erro ao carregar chave privada do Pagamento: %w",
			err,
		)
	}

	publicKeys, err := cryptography.LoadPublicKeyRegistry(
		map[string]string{
			events.ProducerStock: "keys/ms-pagamento/public_keys/ms-estoque.pem",
		},
	)
	if err != nil {
		channel.Close()
		connection.Close()

		return nil, fmt.Errorf(
			"erro ao carregar chaves públicas do Pagamento: %w",
			err,
		)
	}

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Exchanges declaradas")

	paymentQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueuePayment,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila pagamento declarada")

	for _, routingKey := range []string{
		events.OrderStockConfirmed,
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

	runtime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  paymentQueue.Name,
		PrivateKey: privateKey,
		PublicKeys: publicKeys,
	}

	return runtime, nil
}

func ShouldApprovePayment() bool {
	/*
		True = Pagamento aprovado (85%)
		False = Pagamento recusado (15%)
	*/
	return rand.Float64() > 0.15
}

func HandlePaymentEvent(
	envelope events.EventEnvelope,
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
) error {
	var payload events.OrderReferencePayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf(
			"erro ao desserializar pedido.estoque_ok: %w",
			err,
		)
	}

	switch envelope.EventType {
	case events.OrderStockConfirmed:
		if ShouldApprovePayment() {
			err := PublishPaymentApproved(
				channel,
				privateKey,
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
			if err := PublishPaymentRefused(
				channel,
				privateKey,
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

func PublishPaymentApproved(
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
	orderID string,
) error {
	payload := events.PaymentResultPayload{
		OrderID: orderID,
	}

	envelope, err := misc.BuildSignedEnvelope(
		payload,
		events.PaymentApproved,
		events.ProducerPayment,
		privateKey,
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PaymentApproved,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}

func PublishPaymentRefused(
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
	orderID string,
	reason string,
) error {
	payload := events.PaymentResultPayload{
		OrderID: orderID,
		Reason:  reason,
	}

	envelope, err := misc.BuildSignedEnvelope(
		payload,
		events.PaymentRefused,
		events.ProducerPayment,
		privateKey,
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PaymentRefused,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}
