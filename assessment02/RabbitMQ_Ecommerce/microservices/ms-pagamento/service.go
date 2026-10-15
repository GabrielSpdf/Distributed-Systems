package mspagamento

import (
	"crypto/rsa"
	"fmt"
	"log"
	"sync"

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

// SignedPublisher signs each event as ms-pagamento and sends it to the
// eCommerce exchange. The consumer and the webhook handlers share the channel,
// so publications are serialized.
type SignedPublisher struct {
	mu         sync.Mutex
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
		events.ProducerPayment,
		publisher.privateKey,
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	publisher.mu.Lock()
	defer publisher.mu.Unlock()

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
