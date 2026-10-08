package orders

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

const validStockConfirmedJSON = `{
	"order_id":"PED-001",
	"customer_id":"cliente-teste",
	"total":59.8,
	"currency":"BRL",
	"reserved_at":"2026-10-07T20:00:00Z"
}`

func TestReadStockConfirmedPayloadAcceptsValidData(t *testing.T) {
	payload, err := ReadStockConfirmedPayload(
		json.RawMessage(validStockConfirmedJSON),
	)
	if err != nil {
		t.Fatalf("payload válido foi rejeitado: %v", err)
	}

	if payload.OrderID != "PED-001" ||
		payload.CustomerID != "cliente-teste" ||
		payload.Total != 59.8 ||
		payload.Currency != "BRL" {
		t.Fatalf("dados interpretados incorretamente: %+v", payload)
	}

	if payload.ReservedAt.Format("2006-01-02T15:04:05Z07:00") !=
		"2026-10-07T20:00:00Z" {
		t.Fatalf("data interpretada incorretamente: %v", payload.ReservedAt)
	}
}

func TestReadStockConfirmedPayloadRejectsInvalidData(t *testing.T) {
	change := func(old, replacement string) string {
		return strings.Replace(
			validStockConfirmedJSON,
			old,
			replacement,
			1,
		)
	}

	tests := []struct {
		name string
		data string
	}{
		{"JSON incompleto", `{`},
		{"payload nulo", `null`},
		{"campos ausentes", `{}`},
		{"pedido vazio", change(`"PED-001"`, `""`)},
		{"cliente em branco", change(`"cliente-teste"`, `"   "`)},
		{"total zero", change(`59.8`, `0`)},
		{"total negativo", change(`59.8`, `-1`)},
		{"total como texto", change(`59.8`, `"59.8"`)},
		{"moeda incorreta", change(`"BRL"`, `"USD"`)},
		{"data ausente", change(`"reserved_at":"2026-10-07T20:00:00Z"`, `"outro_campo":true`)},
		{"data inválida", change(`"2026-10-07T20:00:00Z"`, `"ontem"`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ReadStockConfirmedPayload(
				json.RawMessage(test.data),
			)
			if err == nil {
				t.Fatal("payload inválido deveria ser rejeitado")
			}
		})
	}
}

func TestValidateOrderDeletedPayload(t *testing.T) {
	for _, reason := range []string{
		events.ReasonCustomerCancelled,
		events.ReasonPaymentRefused,
		events.ReasonCheckoutExpired,
		events.ReasonInternalFailure,
	} {
		t.Run(reason, func(t *testing.T) {
			payload := events.WebOrderDeletedPayload{
				OrderID:     "PED-001",
				CustomerID:  "c537d2d6-f125-4119-965e-3854562c8729",
				Reason:      reason,
				CancelledAt: time.Now().UTC(),
			}
			if err := ValidateOrderDeletedPayload(payload); err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
		})
	}

	tests := []struct {
		name   string
		change func(*events.WebOrderDeletedPayload)
	}{
		{"pedido vazio", func(p *events.WebOrderDeletedPayload) {
			p.OrderID = ""
		}},
		{"pedido em branco", func(p *events.WebOrderDeletedPayload) {
			p.OrderID = "   "
		}},
		{"pedido muito longo", func(p *events.WebOrderDeletedPayload) {
			p.OrderID = strings.Repeat("X", 33)
		}},
		{"cliente vazio", func(p *events.WebOrderDeletedPayload) {
			p.CustomerID = ""
		}},
		{"cliente inválido", func(p *events.WebOrderDeletedPayload) {
			p.CustomerID = "cliente-teste"
		}},
		{"motivo vazio", func(p *events.WebOrderDeletedPayload) {
			p.Reason = ""
		}},
		{"motivo desconhecido", func(p *events.WebOrderDeletedPayload) {
			p.Reason = "OUTRO"
		}},
		{"data ausente", func(p *events.WebOrderDeletedPayload) {
			p.CancelledAt = time.Time{}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := events.WebOrderDeletedPayload{
				OrderID:     "PED-001",
				CustomerID:  "c537d2d6-f125-4119-965e-3854562c8729",
				Reason:      events.ReasonCustomerCancelled,
				CancelledAt: time.Now().UTC(),
			}
			tt.change(&payload)

			if err := ValidateOrderDeletedPayload(payload); err == nil {
				t.Fatal("esperava rejeição do payload inválido")
			}
		})
	}
}
