package orders

import (
	"net/http"
	"testing"
	"time"
)

func TestOrderCheckoutLink(t *testing.T) {
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	zero := time.Time{}

	tests := []struct {
		name   string
		status string
		url    string
		expiry *time.Time
		want   bool
	}{
		{"checkout HTTPS válido", StatusAwaitingPayment,
			"https://checkout.example.com/charge", &future, true},
		{"mock HTTP local", StatusAwaitingPayment,
			"http://localhost:8083/checkout/charge", &future, true},
		{"expirado", StatusAwaitingPayment,
			"https://checkout.example.com/charge", &past, false},
		{"validade ausente", StatusAwaitingPayment,
			"https://checkout.example.com/charge", nil, false},
		{"validade zero", StatusAwaitingPayment,
			"https://checkout.example.com/charge", &zero, false},
		{"URL vazia", StatusAwaitingPayment, "", &future, false},
		{"URL relativa", StatusAwaitingPayment,
			"/checkout/charge", &future, false},
		{"URL inválida", StatusAwaitingPayment,
			"http://%", &future, false},
		{"esquema inseguro", StatusAwaitingPayment,
			"javascript:alert(1)", &future, false},
		{"URL com credenciais", StatusAwaitingPayment,
			"https://user:password@example.com/checkout", &future, false},
	}

	for _, status := range []string{
		StatusPending,
		StatusStockConfirmed,
		StatusCancelled,
		StatusStockUnavailable,
		StatusPaymentRefused,
		StatusPaymentApproved,
		StatusShipped,
		StatusProcessingFailed,
		"DESCONHECIDO",
	} {
		tests = append(tests, struct {
			name   string
			status string
			url    string
			expiry *time.Time
			want   bool
		}{
			name:   "estado " + status,
			status: status,
			url:    "https://checkout.example.com/charge",
			expiry: &future,
			want:   false,
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := Order{
				ID:                "PED-TESTE",
				Status:            test.status,
				CheckoutURL:       test.url,
				CheckoutExpiresAt: test.expiry,
				// Um link antigo deve desaparecer se não for mais permitido.
				Links: map[string]Link{
					"checkout": {Href: "https://old.example.com"},
				},
			}

			addOrderLinks(&order)

			link, exists := order.Links["checkout"]
			if exists != test.want {
				t.Fatalf("checkout presente=%v; esperado=%v", exists, test.want)
			}
			if exists && (link.Href != test.url || link.Method != http.MethodGet) {
				t.Fatalf("link incorreto: %+v", link)
			}

			self := order.Links["self"]
			if self.Href != "/api/orders/PED-TESTE" ||
				self.Method != http.MethodGet {
				t.Fatal("link self incorreto")
			}

			_, canCancel := order.Links["cancel"]
			wantCancel := test.status == StatusPending ||
				test.status == StatusStockConfirmed
			if canCancel != wantCancel {
				t.Fatal("regra de cancelamento foi alterada")
			}
		})
	}
}
