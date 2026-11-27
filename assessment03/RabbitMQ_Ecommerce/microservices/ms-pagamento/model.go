package mspagamento

import (
	"errors"
	"time"
)

const (
	ChargePending  = "PENDING"
	ChargeApproved = "APPROVED"
	ChargeRefused  = "REFUSED"
	// ChargeExpired: prazo do checkout vencido sem decisão. O Pagamento é a
	// autoridade: depois disso nenhum webhook aprova a cobrança.
	ChargeExpired = "EXPIRED"
)

var (
	ErrChargeNotFound     = errors.New("cobrança não encontrada")
	ErrInvalidWebhook     = errors.New("webhook inválido")
	ErrConflictingResult  = errors.New("resultado conflitante para a cobrança")
	ErrWebhookEventReused = errors.New("event_id do webhook já usado em outra cobrança")
	ErrCheckoutExpired    = errors.New("decisão tomada depois do prazo do checkout")
	ErrInvalidStockResult = errors.New("pedido.estoque_ok inválido")
	ErrConflictingCharge  = errors.New("pedido já possui cobrança com dados diferentes")
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
	CheckoutEnvelope  string
	ResultEnvelope    string
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
