package msentrega

import (
	"errors"
	"time"
)

var ErrInvalidPayment = errors.New("pagamento.aprovado inválido")

// Shipment is one row of entrega.shipments: the single delivery of an order.
// Envelope is the signed pedido.enviado, stored as text so it can be resent
// byte for byte; Published turns true after the RabbitMQ confirmation.
type Shipment struct {
	OrderID      string
	CustomerID   string
	ChargeID     string
	InvoiceID    string
	TrackingCode string
	ShippedAt    time.Time
	Envelope     string
	Published    bool
}
