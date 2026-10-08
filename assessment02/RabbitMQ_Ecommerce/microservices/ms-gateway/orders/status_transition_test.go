package orders

import (
	"testing"

	"RabbitMQ_Ecommerce/utils/events"
)

func TestNextOrderStatusAllowsValidTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		event   string
		want    string
	}{
		{"estoque confirmado", StatusPending, events.OrderStockConfirmed, StatusStockConfirmed},
		{"estoque indisponível", StatusPending, events.StockUnavailable, StatusStockUnavailable},
		{"checkout disponível", StatusStockConfirmed, events.PaymentCheckoutAvailable, StatusAwaitingPayment},
		{"pagamento aprovado", StatusAwaitingPayment, events.PaymentApproved, StatusPaymentApproved},
		{"pagamento recusado", StatusAwaitingPayment, events.PaymentRefused, StatusPaymentRefused},
		{"pedido enviado", StatusPaymentApproved, events.OrderShipped, StatusShipped},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NextOrderStatus(test.current, test.event)
			if err != nil {
				t.Fatalf("transição deveria ser permitida: %v", err)
			}

			if got != test.want {
				t.Fatalf("estado obtido %s; esperado %s", got, test.want)
			}
		})
	}
}

func TestNextOrderStatusRejectsInvalidTransitions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		event   string
	}{
		{"pagamento antes do checkout", StatusPending, events.PaymentApproved},
		{"envio antes do pagamento", StatusAwaitingPayment, events.OrderShipped},
		{"regressão após aprovação", StatusPaymentApproved, events.PaymentCheckoutAvailable},
		{"pedido cancelado", StatusCancelled, events.OrderStockConfirmed},
		{"estoque indisponível", StatusStockUnavailable, events.OrderStockConfirmed},
		{"pagamento recusado", StatusPaymentRefused, events.PaymentApproved},
		{"pedido enviado", StatusShipped, events.PaymentRefused},
		{"pedido com falha", StatusProcessingFailed, events.OrderStockConfirmed},
		{"evento desconhecido", StatusPending, "evento.desconhecido"},
		{"estado desconhecido", "ESTADO_DESCONHECIDO", events.OrderStockConfirmed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NextOrderStatus(test.current, test.event)
			if err == nil {
				t.Fatal("transição deveria ser rejeitada")
			}

			if got != "" {
				t.Fatalf("erro não deveria retornar novo estado: %s", got)
			}
		})
	}
}
