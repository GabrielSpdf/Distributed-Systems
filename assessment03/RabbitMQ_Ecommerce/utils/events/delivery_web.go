package events

import "time"

// WebOrderShippedPayload representa pedido.enviado.
type WebOrderShippedPayload struct {
	OrderID      string    `json:"order_id"`
	CustomerID   string    `json:"customer_id"`
	InvoiceID    string    `json:"invoice_id"`
	TrackingCode string    `json:"tracking_code"`
	ShippedAt    time.Time `json:"shipped_at"`
}
