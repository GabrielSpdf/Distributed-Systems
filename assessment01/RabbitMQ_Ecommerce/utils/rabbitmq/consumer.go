package rabbitmq

import (
	"encoding/json"
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConsumeEvents recebe mensagens da fila e encaminha cada envelope
// desserializado ao handler informado.
func ConsumeEvents(
	channel *amqp.Channel,
	queueName string,
	handler func(envelope events.EventEnvelope) error,
) error {
	deliveries, err := channel.Consume(
		queueName, // queue
		"",        // consumer
		false,     // autoAck
		false,     // exclusive
		false,     // noLocal
		false,     // noWait
		nil,       // arguments
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao iniciar consumo da fila %s: %w",
			queueName,
			err,
		)
	}

	for delivery := range deliveries {
		var envelope events.EventEnvelope

		if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
			fmt.Printf(
				"falha ao decodificar mensagem da fila %s: %v\n",
				queueName,
				err,
			)

			if nackErr := delivery.Nack(false, false); nackErr != nil {
				return fmt.Errorf(
					"erro ao rejeitar mensagem inválida: %w",
					nackErr,
				)
			}

			continue
		}

		if err := handler(envelope); err != nil {
			fmt.Printf(
				"falha ao processar evento %s: %v\n",
				envelope.EventID,
				err,
			)

			if nackErr := delivery.Nack(false, false); nackErr != nil {
				return fmt.Errorf(
					"erro ao rejeitar evento %s: %w",
					envelope.EventID,
					nackErr,
				)
			}

			continue
		}

		if err := delivery.Ack(false); err != nil {
			return fmt.Errorf(
				"erro ao confirmar evento %s: %w",
				envelope.EventID,
				err,
			)
		}
	}

	return nil
}
