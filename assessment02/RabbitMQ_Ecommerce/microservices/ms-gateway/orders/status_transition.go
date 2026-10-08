package orders

import (
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"
)

func NextOrderStatus(
	currentStatus string,
	eventType string,
) (string, error) {
	var expectedStatus, nextStatus string

	switch eventType {
	case events.OrderStockConfirmed:
		expectedStatus = StatusPending
		nextStatus = StatusStockConfirmed

	case events.StockUnavailable:
		expectedStatus = StatusPending
		nextStatus = StatusStockUnavailable

	case events.PaymentCheckoutAvailable:
		expectedStatus = StatusStockConfirmed
		nextStatus = StatusAwaitingPayment

	case events.PaymentApproved:
		expectedStatus = StatusAwaitingPayment
		nextStatus = StatusPaymentApproved

	case events.PaymentRefused:
		expectedStatus = StatusAwaitingPayment
		nextStatus = StatusPaymentRefused

	case events.OrderShipped:
		expectedStatus = StatusPaymentApproved
		nextStatus = StatusShipped

	default:
		return "", fmt.Errorf(
			"evento não tratado para atualização do pedido: %s",
			eventType,
		)
	}

	if currentStatus != expectedStatus {
		return "", fmt.Errorf(
			"evento %s não permitido no estado %s",
			eventType,
			currentStatus,
		)
	}

	return nextStatus, nil
}
