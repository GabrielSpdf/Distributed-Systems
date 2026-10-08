package events

import "time"

// WebStockConfirmedPayload representa pedido.estoque_ok.
type WebStockConfirmedPayload struct {
	OrderID    string    `json:"order_id"`
	CustomerID string    `json:"customer_id"`
	Total      float64   `json:"total"`
	Currency   string    `json:"currency"`
	ReservedAt time.Time `json:"reserved_at"`
}

// WebUnavailableItem identifica um produto com estoque insuficiente.
type WebUnavailableItem struct {
	ProductID         string `json:"product_id"`
	RequestedQuantity int    `json:"requested_quantity"`
	AvailableQuantity int    `json:"available_quantity"`
}

// WebStockUnavailablePayload representa estoque.indisponivel.
type WebStockUnavailablePayload struct {
	OrderID          string               `json:"order_id"`
	CustomerID       string               `json:"customer_id"`
	UnavailableItems []WebUnavailableItem `json:"unavailable_items"`
	Reason           string               `json:"reason"`
}
