package cryptography_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"strings"
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
		EventType: events.OrderCreated,
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
		EventType: events.OrderCreated,
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

func TestVerifyEnvelopeFromProducerRejectsUnauthorizedEventType(
	t *testing.T,
) {
	// Gera um par de chaves temporário para representar o Estoque.
	stockPrivateKey, err := rsa.GenerateKey(
		rand.Reader,
		cryptography.RSAKeySize,
	)
	if err != nil {
		t.Fatalf(
			"erro ao gerar chave privada do Estoque: %v",
			err,
		)
	}

	// O payload corresponde a um resultado de pagamento.
	payloadJSON, err := json.Marshal(
		events.PaymentResultPayload{
			OrderID: "PED-001",
		},
	)
	if err != nil {
		t.Fatalf(
			"erro ao serializar payload: %v",
			err,
		)
	}

	// Simula o Estoque tentando publicar pagamento.aprovado.
	envelope := events.EventEnvelope{
		EventID:   "EVENT-TEST-UNAUTHORIZED-PRODUCER",
		EventType: events.PaymentApproved,
		Producer:  events.ProducerStock,
		Timestamp: time.Now().UTC(),
		Payload:   payloadJSON,
	}

	// A assinatura é realmente produzida pela chave privada do Estoque.
	if err := cryptography.SignEnvelope(
		&envelope,
		stockPrivateKey,
	); err != nil {
		t.Fatalf(
			"erro ao assinar envelope: %v",
			err,
		)
	}

	// Confirma que a assinatura é matematicamente válida.
	if err := cryptography.VerifyEnvelope(
		envelope,
		&stockPrivateKey.PublicKey,
	); err != nil {
		t.Fatalf(
			"a assinatura do Estoque deveria ser matematicamente válida: %v",
			err,
		)
	}

	publicKeys := cryptography.PublicKeyRegistry{
		events.ProducerStock: &stockPrivateKey.PublicKey,
	}

	// Mesmo com uma assinatura válida, o Estoque não pode publicar
	// um evento que pertence ao Pagamento.
	err = cryptography.VerifyEnvelopeFromProducer(
		envelope,
		publicKeys,
	)
	if err == nil {
		t.Fatal(
			"evento pagamento.aprovado publicado pelo Estoque foi aceito indevidamente",
		)
	}

	if !strings.Contains(
		err.Error(),
		"não está autorizado",
	) {
		t.Fatalf(
			"erro inesperado ao validar autorização do produtor: %v",
			err,
		)
	}
}
