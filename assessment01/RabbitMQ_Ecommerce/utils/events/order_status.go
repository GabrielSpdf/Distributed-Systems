package events

// CanTransitionOrderStatus reports whether an order status transition is valid.
func CanTransitionOrderStatus(
	currentStatus OrderStatus,
	nextStatus OrderStatus,
) bool {
	if currentStatus == nextStatus {
		return true
	}

	switch currentStatus {
	case StatusPending:
		return nextStatus == StatusStockReserved ||
			nextStatus == StatusStockUnavailable ||
			nextStatus == StatusProcessingFailed

	case StatusStockReserved:
		return nextStatus == StatusPaymentApproved ||
			nextStatus == StatusPaymentRefused

	case StatusPaymentApproved:
		return nextStatus == StatusShipped

	case StatusStockUnavailable,
		StatusPaymentRefused,
		StatusShipped,
		StatusProcessingFailed:
		return false

	default:
		return false
	}
}
