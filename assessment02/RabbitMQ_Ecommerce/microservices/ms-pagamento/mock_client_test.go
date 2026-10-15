package mspagamento

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestMockClient(url string) *MockClient {
	return NewMockClient(MockClientOptions{
		BaseURL:    url,
		WebhookURL: "http://localhost:8082/webhooks/payments",
		RetryWait:  time.Millisecond,
	})
}

const mockChargeJSON = `{
	"charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
	"order_id": "PED-001",
	"status": "PENDING",
	"checkout_url": "http://localhost:8083/checkout/d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
	"expires_at": "2026-10-01T18:31:00Z"
}`

func TestMockClientSendsChargeRequest(t *testing.T) {
	var (
		received       map[string]any
		idempotencyKey string
	)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/charges" {
			t.Errorf("requisição inesperada: %s %s", request.Method, request.URL.Path)
		}

		idempotencyKey = request.Header.Get("Idempotency-Key")
		_ = json.NewDecoder(request.Body).Decode(&received)

		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte(mockChargeJSON))
	}))
	defer server.Close()

	charge, err := newTestMockClient(server.URL).CreateCharge(context.Background(), "PED-001", 59.80, "BRL")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if idempotencyKey != "PED-001" {
		t.Fatalf("Idempotency-Key inesperada: %q", idempotencyKey)
	}

	if received["order_id"] != "PED-001" || received["amount"] != 59.80 ||
		received["currency"] != "BRL" ||
		received["webhook_url"] != "http://localhost:8082/webhooks/payments" {
		t.Fatalf("corpo inesperado: %+v", received)
	}

	if charge.ChargeID != "d67af0cb-b7e8-44c7-a035-22c349c2cbc3" ||
		charge.CheckoutURL == "" ||
		!charge.ExpiresAt.Equal(time.Date(2026, 10, 1, 18, 31, 0, 0, time.UTC)) {
		t.Fatalf("cobrança inesperada: %+v", charge)
	}
}

func TestMockClientRetriesServerErrors(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		_, _ = writer.Write([]byte(mockChargeJSON))
	}))
	defer server.Close()

	if _, err := newTestMockClient(server.URL).CreateCharge(context.Background(), "PED-001", 59.80, "BRL"); err != nil {
		t.Fatalf("esperava sucesso na terceira tentativa: %v", err)
	}

	if calls.Load() != 3 {
		t.Fatalf("esperava 3 tentativas, houve %d", calls.Load())
	}
}

func TestMockClientDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	if _, err := newTestMockClient(server.URL).CreateCharge(context.Background(), "PED-001", 59.80, "BRL"); err == nil {
		t.Fatal("esperava erro")
	}

	if calls.Load() != 1 {
		t.Fatalf("erro 400 não deve ser repetido: %d tentativas", calls.Load())
	}
}

func TestMockClientRejectsIncompleteResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"charge_id":"x"}`))
	}))
	defer server.Close()

	if _, err := newTestMockClient(server.URL).CreateCharge(context.Background(), "PED-001", 59.80, "BRL"); err == nil {
		t.Fatal("esperava erro para resposta incompleta")
	}
}
