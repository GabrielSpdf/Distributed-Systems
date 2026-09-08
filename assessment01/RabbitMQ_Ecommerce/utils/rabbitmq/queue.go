package rabbitmq

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

func DeclareQueue(
	channel *amqp.Channel,
	name string,
) (amqp.Queue, error) {
	q, err := channel.QueueDeclare(
		name,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf(
			"erro ao declarar fila %s: %w",
			name,
			err,
		)
	}

	return q, nil
}

func BindQueue(
	channel *amqp.Channel,
	queueName string,
	routingKey string,
	exchangeName string,
) error {
	err := channel.QueueBind(
		queueName,
		routingKey,
		exchangeName,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao vincular fila %s à exchange %s com a chave de roteamento %s: %w",
			queueName,
			exchangeName,
			routingKey,
			err,
		)
	}

	return nil
}
