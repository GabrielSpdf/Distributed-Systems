package orders

import (
	"crypto/rand"
	"crypto/rsa"
	"regexp"
	"testing"

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
