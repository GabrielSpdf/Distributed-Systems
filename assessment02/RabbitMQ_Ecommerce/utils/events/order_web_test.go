package events

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestWebOrderCreatedPayloadJSON(t *testing.T) {
	payload := WebOrderCreatedPayload{
		OrderID:    "PED-001",
		CustomerID: "cliente-teste",
		Items: []WebOrderItem{
			{
				ProductID:   "PROD-001",
				ProductName: "Arroz 5 kg",
				Quantity:    2,
				UnitPrice:   29.90,
			},
		},
		Total:    59.80,
		Currency: "BRL",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("erro ao serializar payload: %v", err)
	}

	const expectedJSON = `{
		"order_id": "PED-001",
		"customer_id": "cliente-teste",
		"items": [{
			"product_id": "PROD-001",
			"product_name": "Arroz 5 kg",
			"quantity": 2,
			"unit_price": 29.90
		}],
		"total": 59.80,
		"currency": "BRL"
	}`

	var actual, expected map[string]any

	if err := json.Unmarshal(data, &actual); err != nil {
		t.Fatalf("erro ao ler JSON gerado: %v", err)
	}

	if err := json.Unmarshal([]byte(expectedJSON), &expected); err != nil {
		t.Fatalf("erro no JSON esperado do teste: %v", err)
	}

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("JSON diferente do contrato: %s", data)
	}
}
