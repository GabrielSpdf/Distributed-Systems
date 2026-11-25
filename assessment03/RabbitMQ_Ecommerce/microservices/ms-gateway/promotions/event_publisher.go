package promotions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishConfirmedInterest(
	ctx context.Context,
	connection *amqp.Connection,
	data []byte,
) error {
	return publishConfirmedInterestToExchange(
		ctx, connection, events.ExchangeEcommerce, data,
	)
}

func publishConfirmedInterestToExchange(
	ctx context.Context,
	connection *amqp.Connection,
	exchangeName string,
	data []byte,
) error {
	if connection == nil || connection.IsClosed() {
		return fmt.Errorf("conexão RabbitMQ indisponível")
	}

	var envelope events.EventEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("envelope inválido: %w", err)
	}
	if envelope.EventID == "" ||
		envelope.EventType != events.PromotionInterest ||
		envelope.Producer != events.ProducerPrincipal ||
		envelope.Signature == "" {
		return fmt.Errorf("envelope de interesse incompleto ou não permitido")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return err
	}

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("erro ao abrir canal: %w", err)
	}
	defer channel.Close()

	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("erro ao ativar confirmações: %w", err)
	}

	returns := channel.NotifyReturn(make(chan amqp.Return, 1))

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(
		ctx,
		exchangeName,
		events.PromotionInterest,
		true, // mandatory: mensagem sem destino deve ser devolvida
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    envelope.EventID,
			Body:         data,
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao publicar interesse: %w", err)
	}
	if confirmation == nil {
		return fmt.Errorf("confirmação não disponível")
	}

	ack, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("erro ao aguardar confirmação: %w", err)
	}
	if !ack {
		return fmt.Errorf("publicação não confirmada pelo RabbitMQ")
	}

	// Uma mensagem sem destino pode receber ACK e também basic.return.
	select {
	case returned, open := <-returns:
		if !open {
			return fmt.Errorf("canal fechado durante a publicação")
		}
		return fmt.Errorf(
			"mensagem devolvida pelo RabbitMQ: %d %s",
			returned.ReplyCode,
			returned.ReplyText,
		)
	default:
		return nil
	}
}
