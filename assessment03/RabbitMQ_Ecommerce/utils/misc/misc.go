package misc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
)

// BuildEnvelope monta um envelope sem assinatura.
func BuildEnvelope(
	payload any,
	eventType string,
	producer string,
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

	// Define a versão 4 e a variante do UUID.
	eventIDBytes[6] = (eventIDBytes[6] & 0x0f) | 0x40
	eventIDBytes[8] = (eventIDBytes[8] & 0x3f) | 0x80

	eventID := fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		eventIDBytes[0:4],
		eventIDBytes[4:6],
		eventIDBytes[6:8],
		eventIDBytes[8:10],
		eventIDBytes[10:16],
	)

	return events.EventEnvelope{
		EventID:   eventID,
		EventType: eventType,
		Producer:  producer,
		Timestamp: time.Now().UTC(),
		Payload:   payloadJSON,
		Signature: "",
	}, nil
}

// BuildSignedEnvelope monta e assina um envelope.
func BuildSignedEnvelope(
	payload any,
	eventType string,
	producer string,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	envelope, err := BuildEnvelope(
		payload,
		eventType,
		producer,
	)
	if err != nil {
		return events.EventEnvelope{}, err
	}

	if err := cryptography.SignEnvelope(
		&envelope,
		privateKey,
	); err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao assinar evento: %w",
			err,
		)
	}

	return envelope, nil
}
