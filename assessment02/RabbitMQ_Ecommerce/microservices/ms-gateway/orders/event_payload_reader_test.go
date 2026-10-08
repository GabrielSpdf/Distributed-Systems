package orders

import (
	"encoding/json"
	"strings"
	"testing"
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