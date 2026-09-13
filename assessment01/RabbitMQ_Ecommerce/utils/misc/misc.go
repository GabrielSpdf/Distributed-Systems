package misc

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
)

// MountEnvelope monta um envelope sem assinatura.
func MountEnvelope(
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

	return events.EventEnvelope{
		EventID:   hex.EncodeToString(eventIDBytes[:]),
		EventType: eventType,
		Producer:  producer,
		Timestamp: time.Now().UTC(),
		Payload:   payloadJSON,
		Signature: "",
	}, nil
}

// MountSignedEnvelope monta e assina um envelope.
func MountSignedEnvelope(
	payload any,
	eventType string,
	producer string,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	envelope, err := MountEnvelope(
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
