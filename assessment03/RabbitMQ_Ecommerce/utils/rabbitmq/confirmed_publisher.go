package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

const confirmTimeout = 5 * time.Second

// PublishConfirmed publica o envelope e só retorna sucesso depois que o
// RabbitMQ confirma o recebimento (publisher confirm) e a mensagem foi
// roteada para pelo menos uma fila (mandatory). A routing key é o tipo do
// evento. Usa um canal próprio, então pode ser chamada de várias goroutines.
func PublishConfirmed(
	ctx context.Context,
	connection *amqp.Connection,
	exchangeName string,
	envelope events.EventEnvelope,
) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("erro ao serializar envelope: %w", err)
	}

	return publishRawConfirmed(ctx, connection, exchangeName, envelope.EventType, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    envelope.EventID,
		Body:         body,
	})
}

// publishRawConfirmed publica uma mensagem pronta com confirm + mandatory.
func publishRawConfirmed(
	ctx context.Context,
	connection *amqp.Connection,
	exchangeName string,
	routingKey string,
	publishing amqp.Publishing,
) error {
	if connection == nil || connection.IsClosed() {
		return fmt.Errorf("conexão RabbitMQ indisponível")
	}

	ctx, cancel := context.WithTimeout(ctx, confirmTimeout)
	defer cancel()

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("erro ao abrir canal de publicação: %w", err)
	}
	defer channel.Close()

	if exchangeName == events.ExchangeEcommerce {
		if err := PrepareEcommerceTopologyForEvent(channel, routingKey); err != nil {
			return fmt.Errorf("erro ao preparar destinos de %s: %w", routingKey, err)
		}
	}

	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("erro ao ativar confirmações: %w", err)
	}

	returns := channel.NotifyReturn(make(chan amqp.Return, 1))

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(
		ctx,
		exchangeName,
		routingKey,
		true,  // mandatory: o broker devolve mensagens sem fila de destino
		false, // immediate
		publishing,
	)
	if err != nil {
		return fmt.Errorf("erro ao publicar %s: %w", routingKey, err)
	}

	if confirmation == nil {
		return fmt.Errorf("confirmação da publicação não disponível")
	}

	ack, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("erro ao aguardar confirmação: %w", err)
	}

	if !ack {
		return fmt.Errorf("publicação de %s não confirmada pelo RabbitMQ", routingKey)
	}

	// Mensagem sem destino recebe ACK e também é devolvida; a biblioteca entrega
	// o basic.return antes da confirmação correspondente.
	select {
	case returned, open := <-returns:
		if !open {
			return fmt.Errorf("canal fechado durante a publicação")
		}

		return fmt.Errorf(
			"%s sem fila de destino: %d %s",
			routingKey,
			returned.ReplyCode,
			returned.ReplyText,
		)
	default:
		return nil
	}
}
