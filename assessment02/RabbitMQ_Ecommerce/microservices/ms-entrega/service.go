package msentrega

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

func InitializeDeliveryService() (
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
			events.ProducerPayment: "keys/ms-entrega/public_keys/ms-pagamento.pem",
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

	if err := rabbitmq.PrepareEcommerceTopology(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, fmt.Errorf("erro ao preparar topologia completa: %w", err)
	}

	deliveryQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueueDelivery,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila entrega declarada")

	for _, routingKey := range []string{
		events.PaymentApproved,
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

// Signer signs outgoing events as ms-entrega.
type Signer struct {
	privateKey *rsa.PrivateKey
}

func NewSigner(privateKey *rsa.PrivateKey) *Signer {
	return &Signer{
		privateKey: privateKey,
	}
}

func (signer *Signer) Sign(eventType string, payload any) (events.EventEnvelope, error) {
	return misc.BuildSignedEnvelope(
		payload,
		eventType,
		events.ProducerDelivery,
		signer.privateKey,
	)
}

// ConfirmedPublisher sends envelopes to the eCommerce exchange and waits for
// the RabbitMQ confirmation.
type ConfirmedPublisher struct {
	connection *amqp.Connection
}

func NewConfirmedPublisher(connection *amqp.Connection) *ConfirmedPublisher {
	return &ConfirmedPublisher{
		connection: connection,
	}
}

func (publisher *ConfirmedPublisher) Publish(
	ctx context.Context,
	envelope events.EventEnvelope,
) error {
	return rabbitmq.PublishConfirmed(
		ctx,
		publisher.connection,
		events.ExchangeEcommerce,
		envelope,
	)
}
