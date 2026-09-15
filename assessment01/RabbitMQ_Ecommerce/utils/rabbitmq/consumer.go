package rabbitmq

import (
	"encoding/json"
	"fmt"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

type EnvelopeValidator func(
	envelope events.EventEnvelope,
) error

// ConsumeEvents recebe mensagens da fila e encaminha cada envelope
// desserializado ao handler informado.
func consumeEvents(
	channel *amqp.Channel,
	queueName string,
	validator EnvelopeValidator,
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

		if delivery.RoutingKey != envelope.EventType {
			fmt.Printf(
				"[SEGURANÇA] Evento %s descartado: routing key %s diferente do tipo %s\n",
				envelope.EventID,
				delivery.RoutingKey,
				envelope.EventType,
			)

			if nackErr := delivery.Nack(false, false); nackErr != nil {
				return fmt.Errorf(
					"erro ao rejeitar evento com routing key incompatível: %w",
					nackErr,
				)
			}

			continue
		}

		if validator != nil {
			if err := validator(envelope); err != nil {
				fmt.Printf(
					"[SEGURANÇA] Evento %s descartado: %v\n",
					envelope.EventID,
					err,
				)

				if nackErr := delivery.Nack(false, false); nackErr != nil {
					return fmt.Errorf(
						"erro ao rejeitar evento com assinatura inválida: %w",
						nackErr,
					)
				}

				continue
			}
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

func ConsumeEvents(
	channel *amqp.Channel,
	queueName string,
	handler func(envelope events.EventEnvelope) error,
) error {
	return consumeEvents(
		channel,
		queueName,
		nil,
		handler,
	)
}

func ConsumeSignedEvents(
	channel *amqp.Channel,
	queueName string,
	publicKeys cryptography.PublicKeyRegistry,
	handler func(envelope events.EventEnvelope) error,
) error {
	validator := func(
		envelope events.EventEnvelope,
	) error {
		return cryptography.VerifyEnvelopeFromProducer(
			envelope,
			publicKeys,
		)
	}

	return consumeEvents(
		channel,
		queueName,
		validator,
		handler,
	)
}
