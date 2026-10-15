package msestoque

import (
	"errors"
	"reflect"
	"testing"
)

func TestAggregateItemsMergesRepeatedProductsAndSorts(t *testing.T) {
	result, err := aggregateItems([]ReservationItem{
		{ProductID: "PROD-002", Quantity: 1},
		{ProductID: "PROD-001", Quantity: 2},
		{ProductID: "PROD-002", Quantity: 3},
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	expected := []ReservationItem{
		{ProductID: "PROD-001", Quantity: 2},
		{ProductID: "PROD-002", Quantity: 4},
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("esperava %+v, recebeu %+v", expected, result)
	}
}

func TestAggregateItemsRejectsInvalidInput(t *testing.T) {
	cases := map[string][]ReservationItem{
		"sem itens":           {},
		"produto vazio":       {{ProductID: "", Quantity: 1}},
		"quantidade zero":     {{ProductID: "PROD-001", Quantity: 0}},
		"quantidade negativa": {{ProductID: "PROD-001", Quantity: -2}},
	}

	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := aggregateItems(items)
			if !errors.Is(err, ErrInvalidOrder) {
				t.Fatalf("esperava ErrInvalidOrder, recebeu %v", err)
			}
		})
	}
}

func TestFindShortfallsReturnsEmptyWhenStockIsEnough(t *testing.T) {
	shortfalls := findShortfalls(
		[]ReservationItem{{ProductID: "PROD-001", Quantity: 2}},
		map[string]int{"PROD-001": 2},
	)

	if shortfalls == nil || len(shortfalls) != 0 {
		t.Fatalf("esperava lista vazia não nula, recebeu %#v", shortfalls)
	}
}

func TestFindShortfallsReportsInsufficientAndUnknownProducts(t *testing.T) {
	shortfalls := findShortfalls(
		[]ReservationItem{
			{ProductID: "PROD-001", Quantity: 5},
			{ProductID: "PROD-002", Quantity: 1},
			{ProductID: "PROD-999", Quantity: 1},
		},
		map[string]int{"PROD-001": 3, "PROD-002": 10},
	)

	expected := []Shortfall{
		{ProductID: "PROD-001", RequestedQuantity: 5, AvailableQuantity: 3},
		{ProductID: "PROD-999", RequestedQuantity: 1, AvailableQuantity: 0},
	}

	if !reflect.DeepEqual(shortfalls, expected) {
		t.Fatalf("esperava %+v, recebeu %+v", expected, shortfalls)
	}
}
