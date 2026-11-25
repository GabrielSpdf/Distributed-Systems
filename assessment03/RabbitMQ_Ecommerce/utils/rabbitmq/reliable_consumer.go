package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

// permanentError marca um erro que não melhora com novas tentativas
// (payload inválido, cliente divergente...).
type permanentError struct {
	err error
}

func (permanent permanentError) Error() string { return permanent.err.Error() }
func (permanent permanentError) Unwrap() error { return permanent.err }

// Permanent indica ao consumidor confiável que a mensagem deve ser rejeitada
// sem novas tentativas. Erros sem essa marca são tratados como temporários.
func Permanent(err error) error {
	if err == nil {
		return nil
	}

	return permanentError{err: err}
}

func IsPermanent(err error) bool {
	var permanent permanentError
	return errors.As(err, &permanent)
}

// RetryOptions controla as novas tentativas de erros temporários.
type RetryOptions struct {
	Attempts  int
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

func (options RetryOptions) withDefaults() RetryOptions {
	if options.Attempts <= 0 {
		options.Attempts = 5
	}

	if options.BaseDelay <= 0 {
		options.BaseDelay = time.Second
	}

	if options.MaxDelay <= 0 {
		options.MaxDelay = 8 * time.Second
	}

	return options
}

// delay dobra a espera a cada tentativa, até MaxDelay (1, 2, 4, 8 s...).
func (options RetryOptions) delay(attempt int) time.Duration {
	wait := options.BaseDelay << (attempt - 1)
	if wait <= 0 || wait > options.MaxDelay {
		return options.MaxDelay
	}

	return wait
}

// ConsumeSignedEventsReliably consome a fila validando JSON, routing key,
// produtor autorizado e assinatura, como ConsumeSignedEvents. A diferença está
// nas falhas do handler:
//
//   - erro marcado com Permanent: a mensagem é rejeitada e registrada;
//   - outro erro (banco, Mock, RabbitMQ): novas tentativas com espera
//     crescente; esgotadas, a mensagem original vai para a fila
//     "<fila>.revisao" e só então é confirmada. Se nem isso der certo, volta
//     para a própria fila (Nack com requeue), sem se perder.
//
// Processa uma mensagem por vez (prefetch 1).
func ConsumeSignedEventsReliably(
	connection *amqp.Connection,
	channel *amqp.Channel,
	queueName string,
	publicKeys cryptography.PublicKeyRegistry,
	handler func(envelope events.EventEnvelope) error,
	options RetryOptions,
) error {
	options = options.withDefaults()

	if err := DeclareReviewQueue(channel, queueName); err != nil {
		return err
	}

	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("erro ao configurar prefetch da fila %s: %w", queueName, err)
	}

	deliveries, err := channel.Consume(queueName, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("erro ao iniciar consumo da fila %s: %w", queueName, err)
	}

	for delivery := range deliveries {
		if err := processDelivery(connection, queueName, publicKeys, handler, options, delivery); err != nil {
			return err
		}
	}

	return nil
}

func processDelivery(
	connection *amqp.Connection,
	queueName string,
	publicKeys cryptography.PublicKeyRegistry,
	handler func(envelope events.EventEnvelope) error,
	options RetryOptions,
	delivery amqp.Delivery,
) error {
	var envelope events.EventEnvelope

	if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
		return reject(delivery, queueName, "", fmt.Sprintf("JSON inválido: %v", err))
	}

	routingKey := originalRoutingKey(delivery)

	if routingKey != envelope.EventType {
		return reject(delivery, queueName, envelope.EventID, fmt.Sprintf(
			"routing key %s diferente do tipo %s",
			routingKey,
			envelope.EventType,
		))
	}

	if err := cryptography.VerifyEnvelopeFromProducer(envelope, publicKeys); err != nil {
		return reject(delivery, queueName, envelope.EventID, err.Error())
	}

	var lastErr error

	for attempt := 1; attempt <= options.Attempts; attempt++ {
		err := handler(envelope)
		if err == nil {
			if ackErr := delivery.Ack(false); ackErr != nil {
				return fmt.Errorf("erro ao confirmar evento %s: %w", envelope.EventID, ackErr)
			}

			return nil
		}

		if IsPermanent(err) {
			return reject(delivery, queueName, envelope.EventID, err.Error())
		}

		lastErr = err

		log.Printf(
			"[AVISO] Fila %s, evento %s: tentativa %d de %d falhou: %v",
			queueName,
			envelope.EventID,
			attempt,
			options.Attempts,
			err,
		)

		if attempt < options.Attempts {
			time.Sleep(options.delay(attempt))
		}
	}

	if err := publishForReview(connection, delivery, queueName, routingKey, lastErr, options.Attempts); err != nil {
		log.Printf(
			"[ERRO] Fila %s, evento %s: não foi possível enviar para revisão (%v); mensagem devolvida à fila",
			queueName,
			envelope.EventID,
			err,
		)

		time.Sleep(options.MaxDelay)

		if nackErr := delivery.Nack(false, true); nackErr != nil {
			return fmt.Errorf("erro ao devolver evento %s à fila: %w", envelope.EventID, nackErr)
		}

		return nil
	}

	log.Printf(
		"[ERRO] Fila %s, evento %s enviado para %s após %d tentativas: %v",
		queueName,
		envelope.EventID,
		ReviewQueueName(queueName),
		options.Attempts,
		lastErr,
	)

	if ackErr := delivery.Ack(false); ackErr != nil {
		return fmt.Errorf("erro ao confirmar evento %s enviado para revisão: %w", envelope.EventID, ackErr)
	}

	return nil
}

// originalRoutingKey devolve a routing key com que o evento foi publicado.
// Uma mensagem reprocessada a partir da fila de revisão chega pela exchange
// padrão (routing key = nome da fila) e carrega a original no cabeçalho.
// Assinatura e produtor continuam sendo verificados em qualquer caso.
func originalRoutingKey(delivery amqp.Delivery) string {
	if delivery.Exchange == "" {
		if original, ok := delivery.Headers[headerOriginRoutingKey].(string); ok && original != "" {
			return original
		}
	}

	return delivery.RoutingKey
}

func reject(delivery amqp.Delivery, queueName string, eventID string, reason string) error {
	log.Printf("[SEGURANÇA] Fila %s, evento %q rejeitado: %s", queueName, eventID, reason)

	if err := delivery.Nack(false, false); err != nil {
		return fmt.Errorf("erro ao rejeitar evento %q: %w", eventID, err)
	}

	return nil
}

func publishForReview(
	connection *amqp.Connection,
	delivery amqp.Delivery,
	queueName string,
	routingKey string,
	cause error,
	attempts int,
) error {
	reason := cause.Error()
	if len(reason) > 500 {
		reason = reason[:500]
	}

	originExchange := delivery.Exchange
	if originExchange == "" {
		if previous, ok := delivery.Headers[headerOriginExchange].(string); ok {
			originExchange = previous
		}
	}

	return publishRawConfirmed(
		context.Background(),
		connection,
		"",
		ReviewQueueName(queueName),
		amqp.Publishing{
			ContentType:  delivery.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    delivery.MessageId,
			Body:         delivery.Body,
			Headers: amqp.Table{
				headerReviewReason:     reason,
				headerReviewAttempts:   int32(attempts),
				headerReviewAt:         time.Now().UTC().Format(time.RFC3339),
				headerOriginQueue:      queueName,
				headerOriginExchange:   originExchange,
				headerOriginRoutingKey: routingKey,
			},
		},
	)
}
