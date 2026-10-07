package orders

import (
	"reflect"
	"testing"

	"RabbitMQ_Ecommerce/utils/events"
)

func TestBuildOrderCreatedPayload(t *testing.T) {
	order := Order{
		DatabaseID: "uuid-interno-do-pedido",
		ID:         "PED-001",
		UserID:     "cliente-teste",
		Status:     StatusPending,
		Items: []OrderItem{
			{
				ProductID:   "PROD-001",
				ProductName: "Arroz 5 kg",
				Quantity:    2,
				UnitPrice:   29.90,
			},
			{
				ProductID:   "PROD-002",
				ProductName: "Detergente",
				Quantity:    3,
				UnitPrice:   4.90,
			},
		},
		Total:    74.50,
		Currency: "BRL",
	}

	expected := events.WebOrderCreatedPayload{
		OrderID:    "PED-001",
		CustomerID: "cliente-teste",
		Items: []events.WebOrderItem{
			{
				ProductID:   "PROD-001",
				ProductName: "Arroz 5 kg",
				Quantity:    2,
				UnitPrice:   29.90,
			},
			{
				ProductID:   "PROD-002",
				ProductName: "Detergente",
				Quantity:    3,
				UnitPrice:   4.90,
			},
		},
		Total:    74.50,
		Currency: "BRL",
	}

	actual := BuildOrderCreatedPayload(order)

	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf(
			"payload diferente do esperado:\nobtido: %+v\nesperado: %+v",
			actual,
			expected,
		)
	}
}
