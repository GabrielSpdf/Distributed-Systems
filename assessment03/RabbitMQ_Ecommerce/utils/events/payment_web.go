package events

import "time"

// WebPaymentCheckoutPayload representa pagamento.checkout_disponivel.
type WebPaymentCheckoutPayload struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	ChargeID    string    `json:"charge_id"`
	CheckoutURL string    `json:"checkout_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// WebPaymentResultPayload representa aprovação ou recusa do pagamento.
type WebPaymentResultPayload struct {
	OrderID     string    `json:"order_id"`
	CustomerID  string    `json:"customer_id"`
	ChargeID    string    `json:"charge_id"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency"`
	Reason      string    `json:"reason,omitempty"`
	ProcessedAt time.Time `json:"processed_at"`
}
