package msentrega

import (
	"RabbitMQ_Ecommerce/utils/events"
	"encoding/json"
)

func paymentSnapshot(payment events.WebPaymentResultPayload) (string, error) {
	cents, err := events.PositiveMoneyCents(payment.Amount)
	if err != nil {
		return "", err
	}
	payment.Amount = float64(cents) / 100
	payment.ProcessedAt = payment.ProcessedAt.UTC()
	encoded, err := json.Marshal(payment)
	return string(encoded), err
}

func sameShipmentInput(a, b Shipment) bool {
	if a.OrderID != b.OrderID || a.CustomerID != b.CustomerID || a.ChargeID != b.ChargeID {
		return false
	}
	if a.PaymentPayload == "" || b.PaymentPayload == "" {
		return a.PaymentPayload == b.PaymentPayload
	}
	var original, incoming events.WebPaymentResultPayload
	if json.Unmarshal([]byte(a.PaymentPayload), &original) != nil ||
		json.Unmarshal([]byte(b.PaymentPayload), &incoming) != nil {
		return false
	}
	originalJSON, originalErr := paymentSnapshot(original)
	incomingJSON, incomingErr := paymentSnapshot(incoming)
	return originalErr == nil && incomingErr == nil && originalJSON == incomingJSON
}
