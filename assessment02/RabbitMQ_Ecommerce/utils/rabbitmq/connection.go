package rabbitmq

import (
	"fmt"

	"RabbitMQ_Ecommerce/utils/config"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Connect() (*amqp.Connection, error) {
	return ConnectURL(config.Load().RabbitMQURL)
}

// ConnectURL opens a RabbitMQ connection using the provided URL.
func ConnectURL(url string) (*amqp.Connection, error) {
	connection, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao conectar ao RabbitMQ: %w",
			err,
		)
	}

	return connection, nil
}

func OpenChannel(connection *amqp.Connection) (*amqp.Channel, error) {
	channel, err := connection.Channel()
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao abrir canal RabbitMQ: %w",
			err,
		)
	}

	return channel, nil
}
