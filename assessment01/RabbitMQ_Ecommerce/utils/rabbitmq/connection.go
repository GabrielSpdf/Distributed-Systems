package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const rabbitMQURL = "amqp://guest:guest@localhost:5672/"

func Connect() (*amqp.Connection, error) {
	connection, err := amqp.Dial(rabbitMQURL)
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
