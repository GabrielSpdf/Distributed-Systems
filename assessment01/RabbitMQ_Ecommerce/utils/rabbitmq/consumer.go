package rabbitmq

import (
	"RabbitMQ_Ecommerce/utils/events"
	"encoding/json"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

func ConsumeEvent(
	channel *amqp.Channel,
	queueName string,
	handler func(envelope events.EventEnvelope) error,
) error {
	msgs, err := channel.Consume(
		queueName, // queue
		"",        // consumer
		false,     // autoAck
		false,     // exclusive
		false,     // noLocal
		false,     // noWait
		nil,       // args
	)

	if err != nil {
		return fmt.Errorf("Erro ao consumir: %w", err)
	}

	for delivery := range msgs {
		var envelope events.EventEnvelope

		if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
			fmt.Printf("Falha ao decodificar: %v\n", err)
			delivery.Nack(false, false) // nao reenvia
			continue
		}

		if err := handler(envelope); err != nil {
			delivery.Nack(false, false) // nao reenvia
			continue
		}
		delivery.Ack(false)
	}

	return nil
}
