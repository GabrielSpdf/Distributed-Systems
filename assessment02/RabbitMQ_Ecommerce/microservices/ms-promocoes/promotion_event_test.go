package mspromocoes

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"testing"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
)

func TestValidatePromotionEvent(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	registry := cryptography.PublicKeyRegistry{
		events.ProducerPromotions: &privateKey.PublicKey,
	}

	valid := events.PromotionPayload{
		ProductID:          "TEST-001",
		ProductName:        "Produto de teste",
		Category:           "alimentos",
		OriginalPrice:      10,
		PromotionalPrice:   9,
		DiscountPercentage: 10,
	}

	build := func(
		payload events.PromotionPayload,
		key string,
	) events.EventEnvelope {
		t.Helper()

		envelope, err := misc.BuildSignedEnvelope(
			payload, key, events.ProducerPromotions, privateKey,
		)
		if err != nil {
			t.Fatal(err)
		}
		return envelope
	}

	for _, tc := range []struct {
		category string
		key      string
	}{
		{"limpeza", events.PromotionCategoryA},
		{"alimentos", events.PromotionCategoryB},
		{"eletronicos", events.PromotionCategoryC},
	} {
		t.Run(tc.category, func(t *testing.T) {
			payload := valid
			payload.Category = tc.category

			got, err := ValidatePromotionEvent(
				build(payload, tc.key), tc.key, registry,
			)
			if err != nil {
				t.Fatal(err)
			}
			if got != payload {
				t.Fatalf("payload diferente: %+v", got)
			}
		})
	}

	reject := func(
		t *testing.T,
		envelope events.EventEnvelope,
		key string,
	) {
		t.Helper()
		_, err := ValidatePromotionEvent(envelope, key, registry)
		if !errors.Is(err, ErrInvalidPromotionEvent) {
			t.Fatalf("esperava rejeição; erro=%v", err)
		}
	}

	t.Run("payload_adulterado", func(t *testing.T) {
		envelope := build(valid, events.PromotionCategoryB)
		envelope.Payload = json.RawMessage(`{"product_name":"Alterado"}`)
		reject(t, envelope, events.PromotionCategoryB)
	})

	t.Run("sem_assinatura", func(t *testing.T) {
		envelope := build(valid, events.PromotionCategoryB)
		envelope.Signature = ""
		reject(t, envelope, events.PromotionCategoryB)
	})

	t.Run("produtor_nao_autorizado", func(t *testing.T) {
		envelope := build(valid, events.PromotionCategoryB)
		envelope.Producer = events.ProducerPrincipal
		if err := cryptography.SignEnvelope(&envelope, privateKey); err != nil {
			t.Fatal(err)
		}
		reject(t, envelope, events.PromotionCategoryB)
	})

	t.Run("categoria_diferente_da_rota", func(t *testing.T) {
		// Assinatura válida, mas Alimentos publicado na rota de Limpeza.
		envelope := build(valid, events.PromotionCategoryA)
		reject(t, envelope, events.PromotionCategoryA)
	})

	t.Run("preco_invalido", func(t *testing.T) {
		payload := valid
		payload.PromotionalPrice = -1
		reject(
			t, build(payload, events.PromotionCategoryB),
			events.PromotionCategoryB,
		)
	})
}
