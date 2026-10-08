package orders

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
)

func TestBuildOrderCreatedEventSignature(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("erro ao gerar chave de teste: %v", err)
	}

	order := Order{
		ID:       "PED-001",
		UserID:   "cliente-teste",
		Total:    59.80,
		Currency: "BRL",
		Items: []OrderItem{
			{
				ProductID:   "PROD-001",
				ProductName: "Arroz 5 kg",
				Quantity:    2,
				UnitPrice:   29.90,
			},
		},
	}

	event, err := BuildOrderCreatedEvent(order, privateKey)
	if err != nil {
		t.Fatalf("erro ao montar evento: %v", err)
	}

	if event.EventType != events.OrderCreated {
		t.Fatalf("tipo de evento incorreto: %s", event.EventType)
	}

	if event.Producer != events.ProducerPrincipal {
		t.Fatalf("produtor incorreto: %s", event.Producer)
	}

	if event.EventID == "" || event.Timestamp.IsZero() {
		t.Fatal("evento sem identificador ou data")
	}

	uuidPattern := regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)

	if !uuidPattern.MatchString(event.EventID) {
		t.Fatalf("identificador não é UUID v4: %s", event.EventID)
	}

	if err := cryptography.VerifyEnvelope(
		event,
		&privateKey.PublicKey,
	); err != nil {
		t.Fatalf("assinatura deveria ser válida: %v", err)
	}

	event.Payload = []byte(`{"order_id":"PED-ALTERADO"}`)

	if err := cryptography.VerifyEnvelope(
		event,
		&privateKey.PublicKey,
	); err == nil {
		t.Fatal("evento alterado deveria ter assinatura inválida")
	}
}

func TestBuildOrderDeletedEventSignature(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	eventID := "ce2f600e-1579-4936-808e-c5368b9d271d"
	payload := events.WebOrderDeletedPayload{
		OrderID:     "PED-001",
		CustomerID:  "c537d2d6-f125-4119-965e-3854562c8729",
		Reason:      events.ReasonCustomerCancelled,
		CancelledAt: time.Now().UTC(),
	}

	envelope, err := BuildOrderDeletedEvent(eventID, payload, privateKey)
	if err != nil {
		t.Fatal(err)
	}

	if envelope.EventID != eventID ||
		envelope.EventType != events.OrderDeleted ||
		envelope.Producer != events.ProducerPrincipal ||
		!envelope.Timestamp.Equal(payload.CancelledAt) {
		t.Fatal("identificação do envelope incorreta")
	}

	var decoded events.WebOrderDeletedPayload
	if err := json.Unmarshal(envelope.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.OrderID != payload.OrderID ||
		decoded.CustomerID != payload.CustomerID ||
		decoded.Reason != payload.Reason ||
		!decoded.CancelledAt.Equal(payload.CancelledAt) {
		t.Fatal("payload do envelope incorreto")
	}

	if err := cryptography.VerifyEnvelope(
		envelope, &privateKey.PublicKey,
	); err != nil {
		t.Fatalf("assinatura deveria ser válida: %v", err)
	}

	decoded.Reason = events.ReasonPaymentRefused
	envelope.Payload, err = json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := cryptography.VerifyEnvelope(
		envelope, &privateKey.PublicKey,
	); err == nil {
		t.Fatal("payload adulterado deveria invalidar a assinatura")
	}

	if _, err := BuildOrderDeletedEvent("", payload, privateKey); err == nil {
		t.Fatal("identificador vazio deveria ser rejeitado")
	}
	if _, err := BuildOrderDeletedEvent(eventID, payload, nil); err == nil {
		t.Fatal("chave ausente deveria ser rejeitada")
	}
}
