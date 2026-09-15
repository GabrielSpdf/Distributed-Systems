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
	case StatusPending, StatusCreated:
		return nextStatus == StatusStockReserved ||
			nextStatus == StatusStockUnavailable ||
			nextStatus == StatusDeleted ||
			nextStatus == StatusProcessingFailed

	case StatusStockReserved:
		return nextStatus == StatusPaymentApproved ||
			nextStatus == StatusPaymentRefused ||
			nextStatus == StatusDeleted

	case StatusPaymentApproved:
		return nextStatus == StatusShipped

	case StatusStockUnavailable,
		StatusPaymentRefused,
		StatusCancelled,
		StatusDeleted,
		StatusShipped,
		StatusProcessingFailed:
		return false

	default:
		return false
	}
}
