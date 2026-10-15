package mspagamento

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	mockAttempts       = 3
	defaultRetryWait   = time.Second
	mockRequestTimeout = 5 * time.Second
)

// MockCharge is the Mock answer to POST /charges (contratos, seção 6).
type MockCharge struct {
	ChargeID    string    `json:"charge_id"`
	OrderID     string    `json:"order_id"`
	Status      string    `json:"status"`
	CheckoutURL string    `json:"checkout_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type MockClientOptions struct {
	BaseURL    string
	WebhookURL string
	RetryWait  time.Duration
	HTTPClient *http.Client
}

// MockClient creates charges in the Mock de Pagamento.
type MockClient struct {
	options MockClientOptions
}

func NewMockClient(options MockClientOptions) *MockClient {
	if options.RetryWait <= 0 {
		options.RetryWait = defaultRetryWait
	}

	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: mockRequestTimeout}
	}

	options.BaseURL = strings.TrimRight(options.BaseURL, "/")

	return &MockClient{
		options: options,
	}
}

type mockChargeRequest struct {
	OrderID    string  `json:"order_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	WebhookURL string  `json:"webhook_url"`
}

// CreateCharge asks the Mock for a charge. The order id is the
// Idempotency-Key, so a repeated call returns the same charge. Network errors
// and 5xx answers are retried; other answers are not.
func (client *MockClient) CreateCharge(
	ctx context.Context,
	orderID string,
	amount float64,
	currency string,
) (MockCharge, error) {
	body, err := json.Marshal(mockChargeRequest{
		OrderID:    orderID,
		Amount:     amount,
		Currency:   currency,
		WebhookURL: client.options.WebhookURL,
	})
	if err != nil {
		return MockCharge{}, fmt.Errorf("erro ao serializar cobrança: %w", err)
	}

	var lastErr error

	for attempt := 1; attempt <= mockAttempts; attempt++ {
		charge, retry, err := client.post(ctx, orderID, body)
		if err == nil {
			return charge, nil
		}

		lastErr = err
		if !retry || attempt == mockAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return MockCharge{}, ctx.Err()
		case <-time.After(client.options.RetryWait * time.Duration(attempt)):
		}
	}

	return MockCharge{}, fmt.Errorf("erro ao criar cobrança no Mock: %w", lastErr)
}

func (client *MockClient) post(
	ctx context.Context,
	orderID string,
	body []byte,
) (MockCharge, bool, error) {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.options.BaseURL+"/charges",
		bytes.NewReader(body),
	)
	if err != nil {
		return MockCharge{}, false, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", orderID)

	response, err := client.options.HTTPClient.Do(request)
	if err != nil {
		return MockCharge{}, true, err
	}
	defer response.Body.Close()

	if response.StatusCode >= 500 {
		return MockCharge{}, true, fmt.Errorf("Mock respondeu HTTP %d", response.StatusCode)
	}

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return MockCharge{}, false, fmt.Errorf("Mock respondeu HTTP %d", response.StatusCode)
	}

	var charge MockCharge
	if err := json.NewDecoder(response.Body).Decode(&charge); err != nil {
		return MockCharge{}, false, fmt.Errorf("resposta inválida do Mock: %w", err)
	}

	if charge.ChargeID == "" || charge.CheckoutURL == "" || charge.ExpiresAt.IsZero() {
		return MockCharge{}, false, fmt.Errorf("resposta incompleta do Mock: %+v", charge)
	}

	return charge, false, nil
}
