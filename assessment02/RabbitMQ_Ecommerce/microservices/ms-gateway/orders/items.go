package orders

import (
	"fmt"
	"strings"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
	"RabbitMQ_Ecommerce/utils/events"
)

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

		lineCents, err := events.OrderItemTotalCents(events.WebOrderItem{
			ProductID: product.ID, ProductName: product.Name,
			Quantity: requested.Quantity, UnitPrice: product.Price,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("produto %s: %w", productID, err)
		}
		if lineCents > events.MaxOrderTotalCents-totalCents {
			return nil, 0, fmt.Errorf("valor do pedido excede o limite permitido")
		}
		totalCents += lineCents
		unitCents := lineCents / int64(requested.Quantity)

		items = append(items, OrderItem{
			ProductID:   product.ID,
			ProductName: product.Name,
			Quantity:    requested.Quantity,
			UnitPrice:   float64(unitCents) / 100,
		})
	}

	return items, totalCents, nil
}
