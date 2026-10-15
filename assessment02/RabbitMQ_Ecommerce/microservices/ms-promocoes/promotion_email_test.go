package mspromocoes

import (
	"math"
	"strings"
	"testing"

	"RabbitMQ_Ecommerce/utils/events"
)

func TestNormalizePromotionCategory(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Alimentos", "alimentos"},
		{"limpeza", "limpeza"},
		{"Eletrônicos", "eletronicos"},
		{"eletronicos", "eletronicos"},
		{" LIMPEZA ", "limpeza"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			got, err := normalizePromotionCategory(test.input)
			if err != nil || got != test.want {
				t.Fatalf("categoria=%q, erro=%v; esperado=%q", got, err, test.want)
			}
		})
	}

	if _, err := normalizePromotionCategory("roupas"); err == nil {
		t.Fatal("categoria desconhecida foi aceita")
	}
}

func TestBuildPromotionEmail(t *testing.T) {
	promotion := events.PromotionPayload{
		ProductID:          "PROD-002",
		ProductName:        "Detergente",
		Category:           "Limpeza",
		DiscountPercentage: 10,
		OriginalPrice:      10,
		PromotionalPrice:   9,
	}

	message, err := BuildPromotionEmail(promotion, "cliente@example.com")
	if err != nil {
		t.Fatal(err)
	}

	if message.To != "cliente@example.com" ||
		message.Subject != "Oferta Event Market: Detergente" ||
		message.HTML != "" {
		t.Fatalf("mensagem incorreta: %+v", message)
	}

	for _, expected := range []string{
		"Detergente",
		"Preço original: R$ 10,00",
		"Preço promocional: R$ 9,00",
		"cancelar seu interesse",
	} {
		if !strings.Contains(message.Text, expected) {
			t.Errorf("conteúdo não contém %q", expected)
		}
	}
}

func TestBuildPromotionEmailRejectsInvalidInput(t *testing.T) {
	valid := events.PromotionPayload{
		ProductID:          "PROD-002",
		ProductName:        "Detergente",
		Category:           "limpeza",
		DiscountPercentage: 10,
		OriginalPrice:      10,
		PromotionalPrice:   9,
	}

	tests := []struct {
		name   string
		change func(*events.PromotionPayload)
	}{
		{"produto ausente", func(p *events.PromotionPayload) {
			p.ProductID = ""
		}},
		{"nome ausente", func(p *events.PromotionPayload) {
			p.ProductName = ""
		}},
		{"quebra de linha no nome", func(p *events.PromotionPayload) {
			p.ProductName = "Produto\nOutro assunto"
		}},
		{"categoria inválida", func(p *events.PromotionPayload) {
			p.Category = "roupas"
		}},
		{"preço original zero", func(p *events.PromotionPayload) {
			p.OriginalPrice = 0
		}},
		{"preço promocional negativo", func(p *events.PromotionPayload) {
			p.PromotionalPrice = -1
		}},
		{"preço promocional maior", func(p *events.PromotionPayload) {
			p.PromotionalPrice = 11
		}},
		{"desconto zero", func(p *events.PromotionPayload) {
			p.DiscountPercentage = 0
		}},
		{"desconto acima de cem", func(p *events.PromotionPayload) {
			p.DiscountPercentage = 101
		}},
		{"preço incompatível com desconto", func(p *events.PromotionPayload) {
			p.PromotionalPrice = 7
		}},
		{"preço NaN", func(p *events.PromotionPayload) {
			p.OriginalPrice = math.NaN()
		}},
		{"desconto infinito", func(p *events.PromotionPayload) {
			p.DiscountPercentage = math.Inf(1)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			promotion := valid
			test.change(&promotion)

			if _, err := BuildPromotionEmail(
				promotion, "cliente@example.com",
			); err == nil {
				t.Fatal("promoção inválida foi aceita")
			}
		})
	}
}
