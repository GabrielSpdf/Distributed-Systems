package mspromocoes

import (
	"encoding/json"
	"errors"
	"fmt"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgtype"
)

var ErrInvalidPromotionEvent = errors.New("evento de promoção inválido")

func ValidatePromotionEvent(
	envelope events.EventEnvelope,
	routingKey string,
	publicKeys cryptography.PublicKeyRegistry,
) (events.PromotionPayload, error) {
	invalid := func(reason string) (events.PromotionPayload, error) {
		return events.PromotionPayload{}, fmt.Errorf(
			"%w: %s", ErrInvalidPromotionEvent, reason,
		)
	}

	switch routingKey {
	case events.PromotionCategoryA,
		events.PromotionCategoryB,
		events.PromotionCategoryC:
	default:
		return invalid("routing key não permitida")
	}

	if envelope.EventType != routingKey {
		return invalid("tipo do evento diferente da routing key")
	}

	var eventID pgtype.UUID
	if err := eventID.Scan(envelope.EventID); err != nil || !eventID.Valid {
		return invalid("identificador inválido")
	}
	if envelope.Timestamp.IsZero() {
		return invalid("data ausente")
	}

	if err := cryptography.VerifyEnvelopeFromProducer(
		envelope, publicKeys,
	); err != nil {
		return invalid(err.Error())
	}

	var payload events.PromotionPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return invalid("payload inválido")
	}

	category, err := normalizePromotionCategory(payload.Category)
	if err != nil {
		return invalid(err.Error())
	}

	expectedKey := map[string]string{
		"limpeza":     events.PromotionCategoryA,
		"alimentos":   events.PromotionCategoryB,
		"eletronicos": events.PromotionCategoryC,
	}[category]

	if routingKey != expectedKey {
		return invalid("categoria diferente da routing key")
	}

	// Reutiliza as validações de produto, preço e desconto.
	// Apenas monta o conteúdo; não envia e-mail.
	if _, err := BuildPromotionEmail(
		payload, "validacao@example.com",
	); err != nil {
		return invalid(err.Error())
	}

	return payload, nil
}
