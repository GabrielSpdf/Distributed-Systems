package msestoque

import (
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"
)

// ReserveStock verifica a disponibilidade, realiza a baixa e registra
// o pedido reservado. Pedidos já reservados não geram uma nova baixa.
func ReserveStock(
	stock map[string]int,
	reservations map[string]events.Order,
	order events.Order,
) error {
	if order.ID == "" {
		return fmt.Errorf("pedido sem identificador")
	}

	if reservations == nil {
		return fmt.Errorf("mapa de reservas não inicializado")
	}

	if _, exists := reservations[order.ID]; exists {
		return nil
	}

	if len(order.Items) == 0 {
		return fmt.Errorf(
			"nenhum item fornecido para o pedido %s",
			order.ID,
		)
	}

	groupedItems := make(map[string]events.OrderItem, len(order.Items))
	itemOrder := make([]string, 0, len(order.Items))

	// Agrupa produtos repetidos sem alterar o estoque.
	for _, item := range order.Items {
		if item.ProductID == "" {
			return fmt.Errorf("produto sem identificador no pedido %s", order.ID)
		}

		if item.Quantity <= 0 {
			return fmt.Errorf(
				"quantidade inválida para o produto %s: %d",
				item.ProductID,
				item.Quantity,
			)
		}

		groupedItem, exists := groupedItems[item.ProductID]
		if !exists {
			groupedItems[item.ProductID] = item
			itemOrder = append(itemOrder, item.ProductID)
			continue
		}

		groupedItem.Quantity += item.Quantity
		groupedItems[item.ProductID] = groupedItem
	}

	// Verifica todos os produtos antes de realizar qualquer baixa.
	for _, productID := range itemOrder {
		item := groupedItems[productID]

		available, exists := stock[productID]
		if !exists {
			return fmt.Errorf(
				"produto %s não encontrado no estoque",
				productID,
			)
		}

		if item.Quantity > available {
			return fmt.Errorf(
				"estoque insuficiente para %s: solicitado %d, disponível %d",
				productID,
				item.Quantity,
				available,
			)
		}
	}

	// Copia os itens para que a reserva mantenha seus próprios dados.
	reservedOrder := order
	reservedOrder.Items = make([]events.OrderItem, len(order.Items))
	copy(reservedOrder.Items, order.Items)
	reservedOrder.Status = events.StatusStockReserved

	// Todos os itens estão disponíveis: realiza a baixa.
	for _, productID := range itemOrder {
		stock[productID] -= groupedItems[productID].Quantity
	}

	reservations[order.ID] = reservedOrder

	return nil
}

// ReleaseStock devolve os itens reservados para um pedido e remove sua reserva.
// Pedidos sem reserva não alteram o estoque.
func ReleaseStock(
	stock map[string]int,
	reservations map[string]events.Order,
	orderID string,
) error {
	if orderID == "" {
		return fmt.Errorf("pedido sem identificador")
	}

	if reservations == nil {
		return fmt.Errorf("mapa de reservas não inicializado")
	}

	reservedOrder, exists := reservations[orderID]
	if !exists {
		return nil
	}

	if stock == nil {
		return fmt.Errorf("mapa de estoque não inicializado")
	}

	// Devolve as quantidades registradas na reserva.
	for _, item := range reservedOrder.Items {
		stock[item.ProductID] += item.Quantity
	}

	delete(reservations, orderID)

	return nil
}

type StockUnavailableError struct {
	ProductID string
	Reason    string
}

func (e *StockUnavailableError) Error() string {
	return e.Reason
}