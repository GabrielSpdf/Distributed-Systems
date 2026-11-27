package rabbitmq

import (
	"context"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	headerReviewReason     = "x-revisao-motivo"
	headerReviewAttempts   = "x-revisao-tentativas"
	headerReviewAt         = "x-revisao-em"
	headerOriginQueue      = "x-fila-origem"
	headerOriginExchange   = "x-exchange-origem"
	headerOriginRoutingKey = "x-routing-key-origem"
	headerReprocessed      = "x-reprocessado"
)

// ReviewQueueName é a fila que guarda as mensagens de uma fila cujas novas
// tentativas se esgotaram.
func ReviewQueueName(queue string) string {
	return queue + ".revisao"
}

// DeclareReviewQueue cria a fila de revisão durável da fila informada.
func DeclareReviewQueue(channel *amqp.Channel, queue string) error {
	if _, err := channel.QueueDeclare(ReviewQueueName(queue), true, false, false, false, nil); err != nil {
		return fmt.Errorf("erro ao declarar fila de revisão de %s: %w", queue, err)
	}

	return nil
}

// ReprocessReview devolve até limit mensagens da fila de revisão para a fila
// original, preservando corpo, identificador e routing key original. Cada
// mensagem só sai da revisão depois da confirmação da nova publicação.
func ReprocessReview(
	ctx context.Context,
	connection *amqp.Connection,
	queue string,
	limit int,
) (int, error) {
	channel, err := connection.Channel()
	if err != nil {
		return 0, fmt.Errorf("erro ao abrir canal: %w", err)
	}
	defer channel.Close()

	review := ReviewQueueName(queue)
	moved := 0

	for moved < limit {
		delivery, ok, err := channel.Get(review, false)
		if err != nil {
			return moved, fmt.Errorf("erro ao ler %s: %w", review, err)
		}

		if !ok {
			break
		}

		headers := amqp.Table{headerReprocessed: true}
		for _, name := range []string{headerOriginExchange, headerOriginRoutingKey} {
			if value, exists := delivery.Headers[name]; exists {
				headers[name] = value
			}
		}

		err = publishRawConfirmed(ctx, connection, "", queue, amqp.Publishing{
			ContentType:  delivery.ContentType,
			DeliveryMode: amqp.Persistent,
			MessageId:    delivery.MessageId,
			Body:         delivery.Body,
			Headers:      headers,
		})
		if err != nil {
			if nackErr := delivery.Nack(false, true); nackErr != nil {
				log.Printf("[ERRO] Devolução à revisão: %v", nackErr)
			}

			return moved, fmt.Errorf("erro ao reprocessar mensagem %s: %w", delivery.MessageId, err)
		}

		if err := delivery.Ack(false); err != nil {
			return moved, fmt.Errorf("erro ao remover mensagem %s da revisão: %w", delivery.MessageId, err)
		}

		moved++
	}

	return moved, nil
}
