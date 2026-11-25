package msestoque

import (
	"errors"
	"fmt"
	"sort"
)

// ErrInvalidOrder marks an order that can never be reserved as sent.
var ErrInvalidOrder = errors.New("pedido inválido")

// aggregateItems validates the items and merges repeated products. The result
// is sorted by product so every transaction locks rows in the same order.
func aggregateItems(items []ReservationItem) ([]ReservationItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("%w: nenhum item informado", ErrInvalidOrder)
	}

	quantities := make(map[string]int, len(items))

	for _, item := range items {
		if item.ProductID == "" {
			return nil, fmt.Errorf("%w: item sem produto", ErrInvalidOrder)
		}

		if item.Quantity <= 0 {
			return nil, fmt.Errorf(
				"%w: quantidade inválida para o produto %s",
				ErrInvalidOrder,
				item.ProductID,
			)
		}

		quantities[item.ProductID] += item.Quantity
	}

	aggregated := make([]ReservationItem, 0, len(quantities))

	for productID, quantity := range quantities {
		aggregated = append(aggregated, ReservationItem{
			ProductID: productID,
			Quantity:  quantity,
		})
	}

	sort.Slice(aggregated, func(i, j int) bool {
		return aggregated[i].ProductID < aggregated[j].ProductID
	})

	return aggregated, nil
}

// findShortfalls lists the products whose stock does not cover the request.
// A product missing from available counts as zero in stock.
func findShortfalls(
	requested []ReservationItem,
	available map[string]int,
) []Shortfall {
	shortfalls := make([]Shortfall, 0)

	for _, item := range requested {
		quantity := available[item.ProductID]

		if item.Quantity > quantity {
			shortfalls = append(shortfalls, Shortfall{
				ProductID:         item.ProductID,
				RequestedQuantity: item.Quantity,
				AvailableQuantity: quantity,
			})
		}
	}

	return shortfalls
}
