package orders

import (
	"encoding/json"
	"testing"

	"RabbitMQ_Ecommerce/utils/events"
)

func TestReadStockUnavailablePayload(t *testing.T) {
	tests := []struct {
		name    string
		change  func(*events.WebStockUnavailablePayload)
		wantErr bool
	}{
		{"estoque parcial", nil, false},
		{"estoque zero", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].AvailableQuantity = 0
		}, false},
		{"pedido vazio", func(p *events.WebStockUnavailablePayload) {
			p.OrderID = ""
		}, true},
		{"cliente vazio", func(p *events.WebStockUnavailablePayload) {
			p.CustomerID = ""
		}, true},
		{"motivo em branco", func(p *events.WebStockUnavailablePayload) {
			p.Reason = "   "
		}, true},
		{"lista vazia", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems = nil
		}, true},
		{"produto vazio", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].ProductID = ""
		}, true},
		{"produto repetido", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems = append(
				p.UnavailableItems,
				p.UnavailableItems[0],
			)
		}, true},
		{"solicitada zero", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].RequestedQuantity = 0
		}, true},
		{"solicitada negativa", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].RequestedQuantity = -1
		}, true},
		{"disponível negativa", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].AvailableQuantity = -1
		}, true},
		{"disponível igual à solicitada", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].AvailableQuantity = 2
		}, true},
		{"disponível maior que a solicitada", func(p *events.WebStockUnavailablePayload) {
			p.UnavailableItems[0].AvailableQuantity = 3
		}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := events.WebStockUnavailablePayload{
				OrderID:    "PED-001",
				CustomerID: "cliente-teste",
				Reason:     "Estoque insuficiente",
				UnavailableItems: []events.WebUnavailableItem{
					{
						ProductID:         "PROD-001",
						RequestedQuantity: 2,
						AvailableQuantity: 1,
					},
				},
			}

			if test.change != nil {
				test.change(&input)
			}

			data, err := json.Marshal(input)
			if err != nil {
				t.Fatalf("erro ao preparar teste: %v", err)
			}

			_, err = ReadStockUnavailablePayload(data)
			if (err != nil) != test.wantErr {
				t.Fatalf("erro obtido: %v; esperava erro: %v", err, test.wantErr)
			}
		})
	}
}

func TestReadStockUnavailablePayloadRejectsInvalidJSON(t *testing.T) {
	for _, data := range []string{`{`, `null`, `[]`} {
		t.Run(data, func(t *testing.T) {
			if _, err := ReadStockUnavailablePayload(
				json.RawMessage(data),
			); err == nil {
				t.Fatal("payload inválido deveria ser rejeitado")
			}
		})
	}
}
