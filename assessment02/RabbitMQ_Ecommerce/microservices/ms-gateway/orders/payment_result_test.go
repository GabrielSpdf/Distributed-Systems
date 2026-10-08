package orders

import (
	"encoding/json"
	"strings"
	"testing"

	"RabbitMQ_Ecommerce/utils/events"
)

func TestReadPaymentResultPayload(t *testing.T) {
	const validJSON = `{
		"order_id": "PED-001",
		"customer_id": "cliente-teste",
		"charge_id": "COB-001",
		"amount": 4.9,
		"currency": "BRL",
		"processed_at": "2026-10-07T21:00:00Z"
	}`

	tests := []struct {
		name      string
		eventType string
		old       string
		new       string
		wantErr   bool
	}{
		{
			name:      "aprovação sem motivo",
			eventType: events.PaymentApproved,
		},
		{
			name:      "recusa com motivo",
			eventType: events.PaymentRefused,
			old:       `"amount": 4.9`,
			new:       `"amount": 4.9, "reason": "Pagamento recusado"`,
		},
		{
			name:      "recusa sem motivo",
			eventType: events.PaymentRefused,
			wantErr:   true,
		},
		{
			name:      "recusa com motivo em branco",
			eventType: events.PaymentRefused,
			old:       `"amount": 4.9`,
			new:       `"amount": 4.9, "reason": "   "`,
			wantErr:   true,
		},
		{
			name:      "evento desconhecido",
			eventType: "evento.desconhecido",
			wantErr:   true,
		},
		{
			name:      "pedido vazio",
			eventType: events.PaymentApproved,
			old:       `"PED-001"`, new: `""`, wantErr: true,
		},
		{
			name:      "cliente vazio",
			eventType: events.PaymentApproved,
			old:       `"cliente-teste"`, new: `"   "`, wantErr: true,
		},
		{
			name:      "cobrança vazia",
			eventType: events.PaymentApproved,
			old:       `"COB-001"`, new: `""`, wantErr: true,
		},
		{
			name:      "valor zero",
			eventType: events.PaymentApproved,
			old:       `"amount": 4.9`, new: `"amount": 0`, wantErr: true,
		},
		{
			name:      "valor negativo",
			eventType: events.PaymentApproved,
			old:       `"amount": 4.9`, new: `"amount": -4.9`, wantErr: true,
		},
		{
			name:      "valor como texto",
			eventType: events.PaymentApproved,
			old:       `"amount": 4.9`, new: `"amount": "4.9"`, wantErr: true,
		},
		{
			name:      "valor fora do limite numérico",
			eventType: events.PaymentApproved,
			old:       `"amount": 4.9`, new: `"amount": 1e999`, wantErr: true,
		},
		{
			name:      "moeda incorreta",
			eventType: events.PaymentApproved,
			old:       `"BRL"`, new: `"USD"`, wantErr: true,
		},
		{
			name:      "data ausente",
			eventType: events.PaymentApproved,
			old:       `"processed_at"`, new: `"campo_desconhecido"`,
			wantErr: true,
		},
		{
			name:      "data inválida",
			eventType: events.PaymentApproved,
			old:       "2026-10-07T21:00:00Z", new: "ontem",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := validJSON
			if tt.old != "" {
				data = strings.Replace(data, tt.old, tt.new, 1)
			}

			payload, err := ReadPaymentResultPayload(
				json.RawMessage(data),
				tt.eventType,
			)

			if (err != nil) != tt.wantErr {
				t.Fatalf("erro = %v; esperado erro = %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if payload.OrderID != "PED-001" ||
					payload.CustomerID != "cliente-teste" ||
					payload.ChargeID != "COB-001" ||
					payload.Amount != 4.9 ||
					payload.Currency != "BRL" ||
					payload.ProcessedAt.IsZero() {
					t.Fatalf("payload inesperado: %+v", payload)
				}

				if tt.eventType == events.PaymentRefused &&
					payload.Reason != "Pagamento recusado" {
					t.Fatalf("motivo inesperado: %q", payload.Reason)
				}
			}
		})
	}
}

func TestReadPaymentResultPayloadRejectsInvalidJSON(t *testing.T) {
	for _, eventType := range []string{
		events.PaymentApproved,
		events.PaymentRefused,
	} {
		t.Run(eventType, func(t *testing.T) {
			for _, data := range []string{"{", "null", "[]", "{}"} {
				t.Run(data, func(t *testing.T) {
					_, err := ReadPaymentResultPayload(
						json.RawMessage(data),
						eventType,
					)
					if err == nil {
						t.Fatal("esperava rejeição do payload inválido")
					}
				})
			}
		})
	}
}
