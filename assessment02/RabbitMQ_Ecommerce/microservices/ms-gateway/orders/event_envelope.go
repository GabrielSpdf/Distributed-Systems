package orders

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"

	"github.com/jackc/pgx/v5/pgtype"
)

// BuildOrderCreatedEvent monta e assina o evento de criação do pedido.
func BuildOrderCreatedEvent(
	order Order,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	payload := BuildOrderCreatedPayload(order)

	return misc.BuildSignedEnvelope(
		payload,
		events.OrderCreated,
		events.ProducerPrincipal,
		privateKey,
	)
}

func BuildOrderDeletedEvent(
	eventID string,
	payload events.WebOrderDeletedPayload,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	if privateKey == nil {
		return events.EventEnvelope{}, fmt.Errorf("chave privada não configurada")
	}

	var identifier pgtype.UUID
	if err := identifier.Scan(eventID); err != nil || !identifier.Valid {
		return events.EventEnvelope{}, fmt.Errorf("identificador do evento inválido")
	}

	if err := ValidateOrderDeletedPayload(payload); err != nil {
		return events.EventEnvelope{}, err
	}

	payload.CancelledAt = payload.CancelledAt.UTC()

	data, err := json.Marshal(payload)
	if err != nil {
		return events.EventEnvelope{}, err
	}

	envelope := events.EventEnvelope{
		EventID:   eventID,
		EventType: events.OrderDeleted,
		Producer:  events.ProducerPrincipal,
		Timestamp: payload.CancelledAt,
		Payload:   data,
	}

	if err := cryptography.SignEnvelope(&envelope, privateKey); err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao assinar pedido.excluido: %w", err,
		)
	}

	return envelope, nil
}
