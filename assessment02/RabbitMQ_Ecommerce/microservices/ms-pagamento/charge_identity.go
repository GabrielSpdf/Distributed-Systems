package mspagamento

import "RabbitMQ_Ecommerce/utils/events"

func sameChargeBusinessData(a, b Charge) bool {
	aCents, aErr := events.PositiveMoneyCents(a.Amount)
	bCents, bErr := events.PositiveMoneyCents(b.Amount)
	return aErr == nil && bErr == nil && aCents == bCents &&
		a.OrderID == b.OrderID && a.CustomerID == b.CustomerID && a.Currency == b.Currency
}
