package orders

import "RabbitMQ_Ecommerce/utils/events"

// BuildOrderCreatedPayload converte o pedido para o contrato do evento.
func BuildOrderCreatedPayload(order Order) events.WebOrderCreatedPayload {
	items := make([]events.WebOrderItem, 0, len(order.Items))

	for _, item := range order.Items {
		items = append(items, events.WebOrderItem{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    item.Quantity,
			UnitPrice:   item.UnitPrice,
		})
	}

	return events.WebOrderCreatedPayload{
		OrderID:    order.ID,
		CustomerID: order.UserID,
		Items:      items,
		Total:      order.Total,
		Currency:   order.Currency,
	}
}
