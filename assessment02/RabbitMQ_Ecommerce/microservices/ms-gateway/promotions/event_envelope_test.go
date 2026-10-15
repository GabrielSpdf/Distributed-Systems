package promotions

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
)

func TestBuildInterestEventSignature(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	eventID := "11111111-1111-4111-8111-111111111111"
	userID := "22222222-2222-4222-8222-222222222222"
	occurredAt := time.Now().UTC()

	registry := cryptography.PublicKeyRegistry{
		events.ProducerPrincipal: &privateKey.PublicKey,
	}

	for _, action := range []string{ActionSubscribe, ActionUnsubscribe} {
		t.Run(action, func(t *testing.T) {
			payload := InterestPayload{
				UserID:     userID,
				Email:      "cliente@example.com",
				Action:     action,
				Categories: []string{"limpeza"},
				OccurredAt: occurredAt,
			}

			envelope, err := BuildInterestEvent(eventID, payload, privateKey)
			if err != nil {
				t.Fatal(err)
			}

			if envelope.EventID != eventID ||
				envelope.EventType != events.PromotionInterest ||
				envelope.Producer != events.ProducerPrincipal ||
				!envelope.Timestamp.Equal(occurredAt) {
				t.Fatalf("envelope incorreto: %+v", envelope)
			}

			if err := cryptography.VerifyEnvelopeFromProducer(
				envelope, registry,
			); err != nil {
				t.Fatal(err)
			}

			// A assinatura deve continuar válida após serialização.
			data, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			var restored events.EventEnvelope
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			if err := cryptography.VerifyEnvelopeFromProducer(
				restored, registry,
			); err != nil {
				t.Fatal(err)
			}

			// Alterar a categoria após assinar deve invalidar o evento.
			payload.Categories = []string{"alimentos"}
			restored.Payload, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := cryptography.VerifyEnvelopeFromProducer(
				restored, registry,
			); err == nil {
				t.Fatal("evento adulterado foi aceito")
			}
		})
	}
}

func TestBuildInterestEventRejectsInvalidInput(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	eventID := "11111111-1111-4111-8111-111111111111"
	valid := InterestPayload{
		UserID:     "22222222-2222-4222-8222-222222222222",
		Email:      "cliente@example.com",
		Action:     ActionSubscribe,
		Categories: []string{"limpeza"},
		OccurredAt: time.Now().UTC(),
	}

	tests := []struct {
		name   string
		change func(*InterestPayload)
	}{
		{"usuário inválido", func(p *InterestPayload) { p.UserID = "inválido" }},
		{"e-mail inválido", func(p *InterestPayload) { p.Email = "inválido" }},
		{"ação inválida", func(p *InterestPayload) { p.Action = "UNKNOWN" }},
		{"categorias ausentes", func(p *InterestPayload) { p.Categories = nil }},
		{"categoria desconhecida", func(p *InterestPayload) {
			p.Categories = []string{"roupas"}
		}},
		{"categorias repetidas", func(p *InterestPayload) {
			p.Categories = []string{"limpeza", "limpeza"}
		}},
		{"data ausente", func(p *InterestPayload) { p.OccurredAt = time.Time{} }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := valid
			test.change(&payload)
			if _, err := BuildInterestEvent(eventID, payload, privateKey); err == nil {
				t.Fatal("entrada inválida foi aceita")
			}
		})
	}

	t.Run("identificador do evento inválido", func(t *testing.T) {
		if _, err := BuildInterestEvent("inválido", valid, privateKey); err == nil {
			t.Fatal("identificador inválido foi aceito")
		}
	})

	t.Run("chave ausente", func(t *testing.T) {
		if _, err := BuildInterestEvent(eventID, valid, nil); err == nil {
			t.Fatal("chave ausente foi aceita")
		}
	})
}
