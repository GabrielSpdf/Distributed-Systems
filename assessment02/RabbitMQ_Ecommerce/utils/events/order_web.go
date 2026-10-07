package events

// WebOrderItem representa um item no contrato da aplicação web.
type WebOrderItem struct {
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

// WebOrderCreatedPayload representa o conteúdo de pedido.criado.
type WebOrderCreatedPayload struct {
	OrderID    string         `json:"order_id"`
	CustomerID string         `json:"customer_id"`
	Items      []WebOrderItem `json:"items"`
	Total      float64        `json:"total"`
	Currency   string         `json:"currency"`
}
