package orders

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func enqueueOrderCreatedTx(ctx context.Context, tx pgx.Tx, order Order) error {
	created := BuildOrderCreatedPayload(order)
	if err := events.ValidateWebOrderCreated(created); err != nil {
		return fmt.Errorf("criação do pedido inválida: %w", err)
	}
	payload, err := json.Marshal(created)
	if err != nil {
		return fmt.Errorf("erro ao serializar criação do pedido: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.event_outbox (order_id, event_type, payload)
		VALUES ($1, $2, $3::jsonb)
	`, order.DatabaseID, events.OrderCreated, string(payload))
	if err != nil {
		return fmt.Errorf("erro ao guardar criação do pedido na outbox: %w", err)
	}
	return nil
}

// BuildStoredOrderCreatedEvent usa o ID e a data persistidos, não gera outro
// evento de negócio a cada tentativa. A assinatura também é salva na outbox.
func BuildStoredOrderCreatedEvent(
	eventID string,
	payload events.WebOrderCreatedPayload,
	createdAt time.Time,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	if privateKey == nil {
		return events.EventEnvelope{}, fmt.Errorf("chave privada não configurada")
	}
	var identifier pgtype.UUID
	if err := identifier.Scan(eventID); err != nil || !identifier.Valid {
		return events.EventEnvelope{}, fmt.Errorf("identificador do evento inválido")
	}
	if createdAt.IsZero() {
		return events.EventEnvelope{}, fmt.Errorf("data de criação ausente")
	}
	if err := events.ValidateWebOrderCreated(payload); err != nil {
		return events.EventEnvelope{}, fmt.Errorf("payload de criação inválido: %w", err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf("payload de criação inválido: %w", err)
	}
	envelope := events.EventEnvelope{
		EventID:   eventID,
		EventType: events.OrderCreated,
		Producer:  events.ProducerPrincipal,
		Timestamp: createdAt.UTC(),
		Payload:   data,
	}
	if err := cryptography.SignEnvelope(&envelope, privateKey); err != nil {
		return events.EventEnvelope{}, fmt.Errorf("erro ao assinar criação: %w", err)
	}
	return envelope, nil
}
