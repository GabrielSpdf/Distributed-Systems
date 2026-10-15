package mspromocoes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrInvalidInterestEvent = errors.New("evento de interesse inválido")

func ValidateInterestEvent(
	envelope events.EventEnvelope,
	routingKey string,
	publicKeys cryptography.PublicKeyRegistry,
) (events.PromotionInterestPayload, error) {
	payload, err := validateInterestEvent(envelope, routingKey, publicKeys)
	if err != nil {
		return events.PromotionInterestPayload{}, fmt.Errorf(
			"%w: %v", ErrInvalidInterestEvent, err,
		)
	}
	return payload, nil
}

func validateInterestEvent(
	envelope events.EventEnvelope,
	routingKey string,
	publicKeys cryptography.PublicKeyRegistry,
) (events.PromotionInterestPayload, error) {
	if routingKey != events.PromotionInterest ||
		envelope.EventType != routingKey {
		return events.PromotionInterestPayload{}, fmt.Errorf(
			"tipo do evento ou routing key não permitido",
		)
	}

	var eventID pgtype.UUID
	if err := eventID.Scan(envelope.EventID); err != nil || !eventID.Valid {
		return events.PromotionInterestPayload{}, fmt.Errorf(
			"identificador do evento inválido",
		)
	}

	if envelope.Timestamp.IsZero() {
		return events.PromotionInterestPayload{}, fmt.Errorf(
			"data do envelope ausente",
		)
	}

	if err := cryptography.VerifyEnvelopeFromProducer(
		envelope, publicKeys,
	); err != nil {
		return events.PromotionInterestPayload{}, err
	}

	var payload events.PromotionInterestPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return payload, fmt.Errorf("payload inválido: %w", err)
	}

	var userID pgtype.UUID
	if err := userID.Scan(payload.UserID); err != nil || !userID.Valid {
		return payload, fmt.Errorf("identificador do usuário inválido")
	}

	address, err := mail.ParseAddress(payload.Email)
	if err != nil || address.Address != payload.Email {
		return payload, fmt.Errorf("e-mail inválido")
	}

	if payload.Action != events.PromotionSubscribe &&
		payload.Action != events.PromotionUnsubscribe {
		return payload, fmt.Errorf("ação inválida")
	}

	if payload.OccurredAt.IsZero() {
		return payload, fmt.Errorf("data do interesse ausente")
	}

	if len(payload.Categories) == 0 || len(payload.Categories) > 3 {
		return payload, fmt.Errorf("informe de uma a três categorias")
	}

	seen := make(map[string]struct{}, len(payload.Categories))
	for _, category := range payload.Categories {
		switch category {
		case "alimentos", "limpeza", "eletronicos":
		default:
			return payload, fmt.Errorf("categoria inválida: %q", category)
		}

		if _, duplicate := seen[category]; duplicate {
			return payload, fmt.Errorf("categoria repetida: %q", category)
		}
		seen[category] = struct{}{}
	}

	return payload, nil
}
