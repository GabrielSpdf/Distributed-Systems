package cryptography_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
)

func TestVerifyEnvelopeRejectsAlteredPayload(
	t *testing.T,
) {
	privateKey, err := rsa.GenerateKey(
		rand.Reader,
		2048,
	)
	if err != nil {
		t.Fatalf("erro ao gerar chave de teste: %v", err)
	}

	payloadJSON, err := json.Marshal(
		events.OrderReferencePayload{
			OrderID: "PED-001",
		},
	)
	if err != nil {
		t.Fatalf("erro ao serializar payload: %v", err)
	}

	envelope := events.EventEnvelope{
		EventID:   "EVENT-TEST-001",
		EventType: events.PedidoCriado,
		Producer:  events.ProducerPrincipal,
		Timestamp: time.Now().UTC(),
		Payload:   payloadJSON,
	}

	if err := cryptography.SignEnvelope(
		&envelope,
		privateKey,
	); err != nil {
		t.Fatalf("erro ao assinar envelope: %v", err)
	}

	// Primeiro confirma que o envelope original é válido.
	if err := cryptography.VerifyEnvelope(
		envelope,
		&privateKey.PublicKey,
	); err != nil {
		t.Fatalf(
			"envelope original deveria ser válido: %v",
			err,
		)
	}

	// Simula a adulteração depois que o produtor assinou.
	tamperedPayload, err := json.Marshal(
		events.OrderReferencePayload{
			OrderID: "PED-999",
		},
	)
	if err != nil {
		t.Fatalf(
			"erro ao serializar payload adulterado: %v",
			err,
		)
	}

	envelope.Payload = tamperedPayload

	// A assinatura antiga não corresponde ao novo payload.
	if err := cryptography.VerifyEnvelope(
		envelope,
		&privateKey.PublicKey,
	); err == nil {
		t.Fatal(
			"envelope adulterado foi aceito indevidamente",
		)
	}
}

func TestEnvelopeRemainsValidAfterJSONTransport(
	t *testing.T,
) {
	privateKey, err := rsa.GenerateKey(
		rand.Reader,
		2048,
	)
	if err != nil {
		t.Fatalf("erro ao gerar chave de teste: %v", err)
	}

	payloadJSON, err := json.Marshal(
		events.OrderReferencePayload{
			OrderID: "PED-001",
		},
	)
	if err != nil {
		t.Fatalf("erro ao serializar payload: %v", err)
	}

	envelope := events.EventEnvelope{
		EventID:   "EVENT-TEST-002",
		EventType: events.PedidoCriado,
		Producer:  events.ProducerPrincipal,
		Timestamp: time.Now().UTC(),
		Payload:   payloadJSON,
	}

	if err := cryptography.SignEnvelope(
		&envelope,
		privateKey,
	); err != nil {
		t.Fatalf("erro ao assinar envelope: %v", err)
	}

	// Simula a serialização realizada antes da publicação.
	messageBody, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf(
			"erro ao serializar envelope para transporte: %v",
			err,
		)
	}

	// Simula a desserialização realizada pelo consumidor.
	var receivedEnvelope events.EventEnvelope

	if err := json.Unmarshal(
		messageBody,
		&receivedEnvelope,
	); err != nil {
		t.Fatalf(
			"erro ao desserializar envelope recebido: %v",
			err,
		)
	}

	// A passagem pelo JSON não deve invalidar a assinatura.
	if err := cryptography.VerifyEnvelope(
		receivedEnvelope,
		&privateKey.PublicKey,
	); err != nil {
		t.Fatalf(
			"envelope deveria permanecer válido após transporte JSON: %v",
			err,
		)
	}

	// Simula uma adulteração ocorrida durante o transporte.
	tamperedPayload, err := json.Marshal(
		events.OrderReferencePayload{
			OrderID: "PED-999",
		},
	)
	if err != nil {
		t.Fatalf(
			"erro ao montar payload adulterado: %v",
			err,
		)
	}

	receivedEnvelope.Payload = tamperedPayload

	// A assinatura foi calculada para PED-001, não PED-999.
	if err := cryptography.VerifyEnvelope(
		receivedEnvelope,
		&privateKey.PublicKey,
	); err == nil {
		t.Fatal(
			"envelope adulterado foi aceito indevidamente",
		)
	}
}