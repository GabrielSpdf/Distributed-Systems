package events

import (
	"testing"
)

func TestCanTransitionOrderStatus(
	t *testing.T,
) {
	tests := []struct {
		name          string
		currentStatus OrderStatus
		nextStatus    OrderStatus
		expected      bool
	}{
		{
			name:          "pending to stock reserved",
			currentStatus: StatusPending,
			nextStatus:    StatusStockReserved,
			expected:      true,
		},
		{
			name:          "stock reserved to payment approved",
			currentStatus: StatusStockReserved,
			nextStatus:    StatusPaymentApproved,
			expected:      true,
		},
		{
			name:          "payment approved to shipped",
			currentStatus: StatusPaymentApproved,
			nextStatus:    StatusShipped,
			expected:      true,
		},
		{
			name:          "shipped cannot return to stock reserved",
			currentStatus: StatusShipped,
			nextStatus:    StatusStockReserved,
			expected:      false,
		},
		{
			name:          "payment approved cannot be deleted",
			currentStatus: StatusPaymentApproved,
			nextStatus:    StatusDeleted,
			expected:      false,
		},
		{
			name:          "repeated status is idempotent",
			currentStatus: StatusStockReserved,
			nextStatus:    StatusStockReserved,
			expected:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := CanTransitionOrderStatus(
				test.currentStatus,
				test.nextStatus,
			)

			if result != test.expected {
				t.Errorf(
					"CanTransitionOrderStatus(%s, %s) = %t; esperado %t",
					test.currentStatus,
					test.nextStatus,
					result,
					test.expected,
				)
			}
		})
	}
}
