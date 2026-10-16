package mspagamento

import (
	"context"
	"crypto/rsa"
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

// ConfirmedEventBus signs events as ms-pagamento and publishes them to the
// eCommerce exchange, waiting for the RabbitMQ confirmation. Each publication
// uses its own channel, so the consumer and the webhook requests can publish
// at the same time.
type ConfirmedEventBus struct {
	connection *amqp.Connection
	privateKey *rsa.PrivateKey
}

func NewConfirmedEventBus(
	connection *amqp.Connection,
	privateKey *rsa.PrivateKey,
) *ConfirmedEventBus {
	return &ConfirmedEventBus{
		connection: connection,
		privateKey: privateKey,
	}
}

func (bus *ConfirmedEventBus) Sign(eventType string, payload any) (events.EventEnvelope, error) {
	return misc.BuildSignedEnvelope(
		payload,
		eventType,
		events.ProducerPayment,
		bus.privateKey,
	)
}

func (bus *ConfirmedEventBus) Publish(ctx context.Context, envelope events.EventEnvelope) error {
	return rabbitmq.PublishConfirmed(
		ctx,
		bus.connection,
		events.ExchangeEcommerce,
		envelope,
	)
}
