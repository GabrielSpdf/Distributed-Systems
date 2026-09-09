package misc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

// MountEnvelope serializa o payload e monta o envelope comum de um evento.
func MountEnvelope(
	payload any,
	eventType string,
	producer string,
	signature string,
) (events.EventEnvelope, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao serializar payload: %w",
			err,
		)
	}

	var eventIDBytes [16]byte

	if _, err := rand.Read(eventIDBytes[:]); err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao gerar identificador do evento: %w",
			err,
		)
	}

	envelope := events.EventEnvelope{
		EventID:   hex.EncodeToString(eventIDBytes[:]),
		EventType: eventType,
		Producer:  producer,
		Timestamp: time.Now(),
		Payload:   payloadJSON,
		Signature: signature,
	}

	return envelope, nil
}