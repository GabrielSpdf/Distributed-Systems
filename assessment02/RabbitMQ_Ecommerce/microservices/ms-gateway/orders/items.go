package orders

import (
	"fmt"
	"math"
	"strings"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
)

const maxTotalCents int64 = 999999999999

func PrepareItems(
	input []CreateOrderItem,
	products []catalog.Product,
) ([]OrderItem, int64, error) {
	if len(input) == 0 {
		return nil, 0, fmt.Errorf("o pedido deve conter pelo menos um item")
	}

	productsByID := make(map[string]catalog.Product)

	for _, product := range products {
		productsByID[product.ID] = product
	}

	seen := make(map[string]bool)
	items := make([]OrderItem, 0, len(input))
	var totalCents int64

	for _, requested := range input {
		productID := strings.TrimSpace(requested.ProductID)

		if productID == "" {
			return nil, 0, fmt.Errorf("identificador do produto é obrigatório")
		}

		if requested.Quantity <= 0 {
			return nil, 0, fmt.Errorf(
				"quantidade de %s deve ser maior que zero",
				productID,
			)
		}

		if seen[productID] {
			return nil, 0, fmt.Errorf(
				"produto %s está repetido no pedido",
				productID,
			)
		}
		seen[productID] = true

		product, found := productsByID[productID]
		if !found {
			return nil, 0, fmt.Errorf(
				"produto %s não encontrado",
				productID,
			)
		}

		priceCents := math.Round(product.Price * 100)

		if math.IsNaN(priceCents) ||
			math.IsInf(priceCents, 0) ||
			priceCents < 0 ||
			priceCents > float64(maxTotalCents) {
			return nil, 0, fmt.Errorf(
				"preço inválido para %s",
				productID,
			)
		}

		unitCents := int64(priceCents)
		quantity := int64(requested.Quantity)

		if unitCents > 0 &&
			quantity > (maxTotalCents-totalCents)/unitCents {
			return nil, 0, fmt.Errorf("valor do pedido excede o limite permitido")
		}

		totalCents += unitCents * quantity

		items = append(items, OrderItem{
			ProductID:   product.ID,
			ProductName: product.Name,
			Quantity:    requested.Quantity,
			UnitPrice:   float64(unitCents) / 100,
		})
	}

	return items, totalCents, nil
}
