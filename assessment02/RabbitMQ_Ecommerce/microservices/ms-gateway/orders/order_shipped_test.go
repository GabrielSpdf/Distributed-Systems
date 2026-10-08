package orders

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadOrderShippedPayload(t *testing.T) {
	const validJSON = `{
		"order_id": "PED-001",
		"customer_id": "cliente-teste",
		"invoice_id": "NF-001",
		"tracking_code": "RASTREIO-001",
		"shipped_at": "2026-10-07T21:00:00Z"
	}`

	tests := []struct {
		name    string
		old     string
		new     string
		wantErr bool
	}{
		{name: "envio válido"},
		{
			name: "pedido vazio",
			old:  `"PED-001"`, new: `""`, wantErr: true,
		},
		{
			name: "cliente em branco",
			old:  `"cliente-teste"`, new: `"   "`, wantErr: true,
		},
		{
			name: "nota fiscal vazia",
			old:  `"NF-001"`, new: `""`, wantErr: true,
		},
		{
			name: "rastreamento em branco",
			old:  `"RASTREIO-001"`, new: `"   "`, wantErr: true,
		},
		{
			name: "data ausente",
			old:  `"shipped_at"`, new: `"campo_desconhecido"`,
			wantErr: true,
		},
		{
			name: "data inválida",
			old:  "2026-10-07T21:00:00Z", new: "ontem",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := validJSON
			if tt.old != "" {
				data = strings.Replace(data, tt.old, tt.new, 1)
			}

			payload, err := ReadOrderShippedPayload(json.RawMessage(data))
			if (err != nil) != tt.wantErr {
				t.Fatalf("erro = %v; esperado erro = %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if payload.OrderID != "PED-001" ||
					payload.CustomerID != "cliente-teste" ||
					payload.InvoiceID != "NF-001" ||
					payload.TrackingCode != "RASTREIO-001" ||
					payload.ShippedAt.IsZero() {
					t.Fatalf("payload inesperado: %+v", payload)
				}
			}
		})
	}
}

func TestReadOrderShippedPayloadRejectsInvalidJSON(t *testing.T) {
	for _, data := range []string{"{", "null", "[]", "{}"} {
		t.Run(data, func(t *testing.T) {
			_, err := ReadOrderShippedPayload(json.RawMessage(data))
			if err == nil {
				t.Fatal("esperava rejeição do payload inválido")
			}
		})
	}
}
