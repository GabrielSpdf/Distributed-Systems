package mspromocoes

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
)

func TestValidateInterestEvent(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	registry := cryptography.PublicKeyRegistry{
		events.ProducerPrincipal: &privateKey.PublicKey,
	}

	valid := events.PromotionInterestPayload{
		UserID:     "22222222-2222-4222-8222-222222222222",
		Email:      "cliente@example.com",
		Action:     events.PromotionSubscribe,
		Categories: []string{"limpeza"},
		OccurredAt: time.Now().UTC(),
	}

	build := func(payload events.PromotionInterestPayload) events.EventEnvelope {
		t.Helper()
		envelope, err := misc.BuildSignedEnvelope(
			payload,
			events.PromotionInterest,
			events.ProducerPrincipal,
			privateKey,
		)
		if err != nil {
			t.Fatal(err)
		}
		return envelope
	}

	for _, action := range []string{
		events.PromotionSubscribe,
		events.PromotionUnsubscribe,
	} {
		t.Run(action, func(t *testing.T) {
			payload := valid
			payload.Action = action

			got, err := ValidateInterestEvent(
				build(payload), events.PromotionInterest, registry,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got.UserID != payload.UserID || got.Action != action {
				t.Fatalf("payload incorreto: %+v", got)
			}
		})
	}

	t.Run("payload adulterado", func(t *testing.T) {
		envelope := build(valid)
		envelope.Payload = json.RawMessage(`{"categories":["alimentos"]}`)

		_, err := ValidateInterestEvent(
			envelope, events.PromotionInterest, registry,
		)
		if !errors.Is(err, ErrInvalidInterestEvent) {
			t.Fatalf("esperava rejeição; erro=%v", err)
		}
	})

	t.Run("produtor não autorizado", func(t *testing.T) {
		envelope := build(valid)
		envelope.Producer = events.ProducerStock
		if err := cryptography.SignEnvelope(&envelope, privateKey); err != nil {
			t.Fatal(err)
		}

		_, err := ValidateInterestEvent(
			envelope, events.PromotionInterest, registry,
		)
		if !errors.Is(err, ErrInvalidInterestEvent) {
			t.Fatalf("esperava rejeição do produtor; erro=%v", err)
		}
	})

	t.Run("routing key incorreta", func(t *testing.T) {
		_, err := ValidateInterestEvent(build(valid), "pedido.criado", registry)
		if !errors.Is(err, ErrInvalidInterestEvent) {
			t.Fatalf("esperava rejeição da rota; erro=%v", err)
		}
	})

	tests := []struct {
		name   string
		change func(*events.PromotionInterestPayload)
	}{
		{"usuário inválido", func(p *events.PromotionInterestPayload) {
			p.UserID = "inválido"
		}},
		{"e-mail inválido", func(p *events.PromotionInterestPayload) {
			p.Email = "inválido"
		}},
		{"ação inválida", func(p *events.PromotionInterestPayload) {
			p.Action = "UNKNOWN"
		}},
		{"data ausente", func(p *events.PromotionInterestPayload) {
			p.OccurredAt = time.Time{}
		}},
		{"categorias ausentes", func(p *events.PromotionInterestPayload) {
			p.Categories = nil
		}},
		{"categoria desconhecida", func(p *events.PromotionInterestPayload) {
			p.Categories = []string{"roupas"}
		}},
		{"categoria repetida", func(p *events.PromotionInterestPayload) {
			p.Categories = []string{"limpeza", "limpeza"}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := valid
			test.change(&payload)

			// Assina corretamente um payload inválido:
			// assinatura válida não dispensa validação dos dados.
			_, err := ValidateInterestEvent(
				build(payload), events.PromotionInterest, registry,
			)
			if !errors.Is(err, ErrInvalidInterestEvent) {
				t.Fatalf("esperava rejeição do payload; erro=%v", err)
			}
		})
	}
}
