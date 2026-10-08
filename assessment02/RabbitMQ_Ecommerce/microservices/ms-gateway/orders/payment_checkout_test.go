package orders

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadPaymentCheckoutPayload(t *testing.T) {
	const validJSON = `{
		"order_id": "PED-001",
		"customer_id": "cliente-teste",
		"charge_id": "COB-001",
		"checkout_url": "https://checkout.example.com/COB-001",
		"expires_at": "2026-10-07T21:00:00Z"
	}`

	tests := []struct {
		name    string
		old     string
		new     string
		wantErr bool
	}{
		{name: "checkout HTTPS válido"},
		{
			name: "checkout HTTP local válido",
			old:  "https://checkout.example.com/COB-001",
			new:  "http://localhost:8082/checkout/COB-001",
		},
		{
			name: "pedido vazio", old: `"PED-001"`,
			new: `""`, wantErr: true,
		},
		{
			name: "cliente vazio", old: `"cliente-teste"`,
			new: `"   "`, wantErr: true,
		},
		{
			name: "cobrança vazia", old: `"COB-001"`,
			new: `""`, wantErr: true,
		},
		{
			name: "URL vazia",
			old:  "https://checkout.example.com/COB-001",
			new:  "", wantErr: true,
		},
		{
			name: "URL relativa",
			old:  "https://checkout.example.com/COB-001",
			new:  "/checkout/COB-001", wantErr: true,
		},
		{
			name: "protocolo proibido",
			old:  "https://checkout.example.com/COB-001",
			new:  "javascript:alert(1)", wantErr: true,
		},
		{
			name: "URL sem host",
			old:  "https://checkout.example.com/COB-001",
			new:  "https:///checkout/COB-001", wantErr: true,
		},
		{
			name:    "URL com credenciais",
			old:     "https://checkout.example.com/COB-001",
			new:     "https://usuario:senha@checkout.example.com/COB-001",
			wantErr: true,
		},
		{
			name: "URL malformada",
			old:  "https://checkout.example.com/COB-001",
			new:  "https://checkout.example.com/%zz", wantErr: true,
		},
		{
			name: "expiração ausente", old: `"expires_at"`,
			new: `"campo_desconhecido"`, wantErr: true,
		},
		{
			name: "expiração inválida",
			old:  "2026-10-07T21:00:00Z",
			new:  "ontem", wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := validJSON
			if tt.old != "" {
				data = strings.Replace(data, tt.old, tt.new, 1)
			}

			payload, err := ReadPaymentCheckoutPayload(json.RawMessage(data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("erro = %v; esperado erro = %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if payload.OrderID != "PED-001" ||
					payload.CustomerID != "cliente-teste" ||
					payload.ChargeID != "COB-001" ||
					payload.ExpiresAt.IsZero() {
					t.Fatalf("payload inesperado: %+v", payload)
				}
			}
		})
	}
}

func TestReadPaymentCheckoutPayloadRejectsInvalidJSON(t *testing.T) {
	for _, data := range []string{"{", "null", "[]", "{}"} {
		t.Run(data, func(t *testing.T) {
			_, err := ReadPaymentCheckoutPayload(json.RawMessage(data))
			if err == nil {
				t.Fatal("esperava rejeição do payload inválido")
			}
		})
	}
}
