package msestoque

import (
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

func InitializeStockService() (
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

	privateKey, err := cryptography.LoadPrivateKey("keys/ms-estoque/private.pem")
	if err != nil {
		channel.Close()
		connection.Close()

		return nil, fmt.Errorf(
			"erro ao carregar chave privada do Estoque: %w",
			err,
		)
	}

	publicKeys, err := cryptography.LoadPublicKeyRegistry(
		map[string]string{
			events.ProducerPrincipal: "keys/ms-estoque/public_keys/ms-principal.pem",
		},
	)
	if err != nil {
		channel.Close()
		connection.Close()

		return nil, fmt.Errorf(
			"erro ao carregar chaves públicas do Estoque: %w",
			err,
		)
	}

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Exchanges declaradas")

	stockQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueueStock,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila estoque declarada")

	for _, routingKey := range []string{
		events.OrderCreated,
		events.OrderDeleted,
	} {
		if err := rabbitmq.BindQueue(
			channel,
			stockQueue.Name,
			routingKey,
			events.ExchangeEcommerce,
		); err != nil {
			channel.Close()
			connection.Close()
			return nil, err
		}
		log.Println("[SUCESSO] Fila estoque ligada à exchange Ecommerce e vinculada ao roteamento", routingKey)
	}

	runtime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  stockQueue.Name,
		PrivateKey: privateKey,
		PublicKeys: publicKeys,
	}

	return runtime, nil
}

// SignedPublisher signs each event as ms-estoque and sends it to the
// eCommerce exchange, using the event type as the routing key.
type SignedPublisher struct {
	channel    *amqp.Channel
	privateKey *rsa.PrivateKey
}

func NewSignedPublisher(
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
) *SignedPublisher {
	return &SignedPublisher{
		channel:    channel,
		privateKey: privateKey,
	}
}

func (publisher *SignedPublisher) Publish(
	eventType string,
	payload any,
) error {
	envelope, err := misc.BuildSignedEnvelope(
		payload,
		eventType,
		events.ProducerStock,
		publisher.privateKey,
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		publisher.channel,
		events.ExchangeEcommerce,
		eventType,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}
