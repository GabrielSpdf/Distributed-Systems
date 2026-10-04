package orders

import (
	"testing"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
)

func TestPrepareItemsCalculatesTotal(t *testing.T) {
	products := []catalog.Product{
		{
			ID:       "PROD-001",
			Name:     "Arroz 5 kg",
			Price:    29.90,
			Quantity: 30,
		},
		{
			ID:       "PROD-002",
			Name:     "Detergente",
			Price:    4.90,
			Quantity: 50,
		},
	}

	input := []CreateOrderItem{
		{ProductID: "PROD-001", Quantity: 2},
		{ProductID: "PROD-002", Quantity: 3},
	}

	items, totalCents, err := PrepareItems(input, products)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}

	if totalCents != 7450 {
		t.Fatalf("esperava 7450 centavos, recebeu %d", totalCents)
	}

	if len(items) != 2 {
		t.Fatalf("esperava dois itens, recebeu %d", len(items))
	}

	if items[0].ProductName != "Arroz 5 kg" ||
		items[0].UnitPrice != 29.90 {
		t.Fatalf("dados inesperados do catálogo: %+v", items[0])
	}
}

func TestPrepareItemsRejectsInvalidInput(t *testing.T) {
	products := []catalog.Product{
		{ID: "PROD-001", Name: "Arroz 5 kg", Price: 29.90},
	}

	tests := []struct {
		name  string
		items []CreateOrderItem
	}{
		{
			name: "pedido vazio",
		},
		{
			name: "identificador vazio",
			items: []CreateOrderItem{
				{ProductID: "", Quantity: 1},
			},
		},
		{
			name: "quantidade zero",
			items: []CreateOrderItem{
				{ProductID: "PROD-001", Quantity: 0},
			},
		},
		{
			name: "quantidade negativa",
			items: []CreateOrderItem{
				{ProductID: "PROD-001", Quantity: -1},
			},
		},
		{
			name: "produto inexistente",
			items: []CreateOrderItem{
				{ProductID: "PROD-999", Quantity: 1},
			},
		},
		{
			name: "produto repetido",
			items: []CreateOrderItem{
				{ProductID: "PROD-001", Quantity: 1},
				{ProductID: "PROD-001", Quantity: 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := PrepareItems(test.items, products)

			if err == nil {
				t.Fatal("esperava rejeição dos itens inválidos")
			}
		})
	}
}

func TestPrepareItemsLeavesStockDecisionToStockService(t *testing.T) {
	products := []catalog.Product{
		{
			ID:       "PROD-001",
			Name:     "Arroz 5 kg",
			Price:    29.90,
			Quantity: 0,
		},
	}

	_, _, err := PrepareItems(
		[]CreateOrderItem{
			{ProductID: "PROD-001", Quantity: 1},
		},
		products,
	)
	if err != nil {
		t.Fatalf("a reserva deve ser decidida pelo estoque: %v", err)
	}
}

func TestPrepareItemsRejectsTotalAboveDatabaseLimit(t *testing.T) {
	products := []catalog.Product{
		{
			ID:    "PROD-001",
			Name:  "Produto de teste",
			Price: 9999999999.99,
		},
	}

	_, _, err := PrepareItems(
		[]CreateOrderItem{
			{ProductID: "PROD-001", Quantity: 2},
		},
		products,
	)
	if err == nil {
		t.Fatal("esperava rejeição do total acima do limite")
	}
}
