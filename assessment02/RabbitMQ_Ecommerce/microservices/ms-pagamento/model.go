package mspagamento

import (
	"errors"
	"time"
)

const (
	ChargePending  = "PENDING"
	ChargeApproved = "APPROVED"
	ChargeRefused  = "REFUSED"
)

var (
	ErrChargeNotFound     = errors.New("cobrança não encontrada")
	ErrInvalidWebhook     = errors.New("webhook inválido")
	ErrConflictingResult  = errors.New("resultado conflitante para a cobrança")
	ErrInvalidStockResult = errors.New("pedido.estoque_ok inválido")
)

// Charge is one row of pagamento.payment_charges. ID is the Mock charge_id.
type Charge struct {
	ID                string
	OrderID           string
	CustomerID        string
	Amount            float64
	Currency          string
	Status            string
	CheckoutURL       string
	ExpiresAt         time.Time
	ProcessedAt       time.Time
	Reason            string
	CheckoutPublished bool
	ResultPublished   bool
}

// WebhookEvent is the verified body received from the Mock.
type WebhookEvent struct {
	EventID    string    `json:"event_id"`
	ChargeID   string    `json:"charge_id"`
	OrderID    string    `json:"order_id"`
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
}

// WebhookOutcome tells whether the webhook changed the charge. Duplicate means
// the same result had already been stored.
type WebhookOutcome struct {
	Charge    Charge
	Duplicate bool
}
