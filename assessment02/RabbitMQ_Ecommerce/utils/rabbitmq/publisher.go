package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishEvent(
	channel *amqp.Channel,
	exchangeName string,
	routingKey string,
	event events.EventEnvelope,
) error {

	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("erro ao serializar evento para JSON: %w", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err = channel.PublishWithContext(
		ctx,
		exchangeName,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         eventData,
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao publicar evento: %w", err)
	}

	return nil
}
