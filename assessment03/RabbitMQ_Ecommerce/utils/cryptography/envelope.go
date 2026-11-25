package cryptography

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

type signableEnvelope struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Producer  string          `json:"producer"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

func envelopeSigningContent(
	envelope events.EventEnvelope,
) ([]byte, error) {
	signable := signableEnvelope{
		EventID:   envelope.EventID,
		EventType: envelope.EventType,
		Producer:  envelope.Producer,
		Timestamp: envelope.Timestamp,
		Payload:   envelope.Payload,
	}

	content, err := json.Marshal(signable)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao serializar conteúdo assinável: %w",
			err,
		)
	}

	return content, nil
}

// SignEnvelope assina os campos de um envelope.
func SignEnvelope(
	envelope *events.EventEnvelope,
	privateKey *rsa.PrivateKey,
) error {
	if envelope == nil {
		return fmt.Errorf("envelope não informado")
	}

	if privateKey == nil {
		return fmt.Errorf("chave privada não informada")
	}

	content, err := envelopeSigningContent(*envelope)
	if err != nil {
		return err
	}

	signature, err := SignContent(
		content,
		privateKey,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao assinar envelope: %w",
			err,
		)
	}

	envelope.Signature = signature

	return nil
}

// VerifyEnvelope verifica a assinatura digital de um envelope.
func VerifyEnvelope(
	envelope events.EventEnvelope,
	publicKey *rsa.PublicKey,
) error {
	if publicKey == nil {
		return fmt.Errorf("chave pública não informada")
	}

	if envelope.Signature == "" {
		return fmt.Errorf("evento não possui assinatura")
	}

	content, err := envelopeSigningContent(envelope)
	if err != nil {
		return err
	}

	if err := VerifyContent(
		content,
		envelope.Signature,
		publicKey,
	); err != nil {
		return fmt.Errorf(
			"assinatura do evento %s inválida: %w",
			envelope.EventID,
			err,
		)
	}

	return nil
}

func VerifyEnvelopeFromProducer(
	envelope events.EventEnvelope,
	publicKeys PublicKeyRegistry,
) error {
	expectedProducer, exists := events.ExpectedProducerFor(
		envelope.EventType,
	)
	if !exists {
		return fmt.Errorf(
			"tipo de evento não autorizado: %s",
			envelope.EventType,
		)
	}

	if envelope.Producer != expectedProducer {
		return fmt.Errorf(
			"produtor %s não está autorizado a publicar o evento %s; produtor esperado: %s",
			envelope.Producer,
			envelope.EventType,
			expectedProducer,
		)
	}

	publicKey, exists := publicKeys[envelope.Producer]
	if !exists {
		return fmt.Errorf(
			"produtor não autorizado: %s",
			envelope.Producer,
		)
	}

	return VerifyEnvelope(
		envelope,
		publicKey,
	)
}
