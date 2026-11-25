package promotions

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/mail"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgtype"
)

// BuildInterestEvent usa o identificador já salvo na outbox.
func BuildInterestEvent(
	eventID string,
	payload InterestPayload,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	if privateKey == nil {
		return events.EventEnvelope{}, fmt.Errorf("chave privada não configurada")
	}

	var eventIdentifier pgtype.UUID
	if err := eventIdentifier.Scan(eventID); err != nil || !eventIdentifier.Valid {
		return events.EventEnvelope{}, fmt.Errorf("identificador do evento inválido")
	}

	var userIdentifier pgtype.UUID
	if err := userIdentifier.Scan(payload.UserID); err != nil || !userIdentifier.Valid {
		return events.EventEnvelope{}, fmt.Errorf("identificador do usuário inválido")
	}

	address, err := mail.ParseAddress(payload.Email)
	if err != nil || address.Address != payload.Email {
		return events.EventEnvelope{}, fmt.Errorf("e-mail inválido")
	}

	if payload.Action != ActionSubscribe && payload.Action != ActionUnsubscribe {
		return events.EventEnvelope{}, fmt.Errorf("ação de interesse inválida")
	}

	if err := validateCategories(payload.Categories); err != nil {
		return events.EventEnvelope{}, err
	}

	if payload.OccurredAt.IsZero() {
		return events.EventEnvelope{}, fmt.Errorf("data do interesse ausente")
	}

	payload.OccurredAt = payload.OccurredAt.UTC()

	data, err := json.Marshal(payload)
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf("erro ao serializar interesse: %w", err)
	}

	envelope := events.EventEnvelope{
		EventID:   eventID,
		EventType: events.PromotionInterest,
		Producer:  events.ProducerPrincipal,
		Timestamp: payload.OccurredAt,
		Payload:   data,
	}

	if err := cryptography.SignEnvelope(&envelope, privateKey); err != nil {
		return events.EventEnvelope{}, fmt.Errorf("erro ao assinar interesse: %w", err)
	}

	return envelope, nil
}
