package mspagamento

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

// stubOutboxStore guarda em memória o que o Outbox grava no banco. Os stubs
// de cobrança e de webhook o incorporam.
type stubOutboxStore struct {
	checkoutEnvelopes []string
	resultEnvelopes   []string
	saveEnvelopeErr   error
	checkoutsMarked   []string
	resultsMarked     []string
	pendingCheckouts  []Charge
	pendingResults    []Charge
}

func (store *stubOutboxStore) SaveCheckoutEnvelope(_ context.Context, _ string, envelope string) (string, error) {
	if store.saveEnvelopeErr != nil {
		return "", store.saveEnvelopeErr
	}

	store.checkoutEnvelopes = append(store.checkoutEnvelopes, envelope)

	return envelope, nil
}

func (store *stubOutboxStore) SaveResultEnvelope(_ context.Context, _ string, envelope string) (string, error) {
	if store.saveEnvelopeErr != nil {
		return "", store.saveEnvelopeErr
	}

	store.resultEnvelopes = append(store.resultEnvelopes, envelope)

	return envelope, nil
}

func (store *stubOutboxStore) MarkCheckoutPublished(_ context.Context, chargeID string) error {
	store.checkoutsMarked = append(store.checkoutsMarked, chargeID)

	return nil
}

func (store *stubOutboxStore) MarkResultPublished(_ context.Context, chargeID string) error {
	store.resultsMarked = append(store.resultsMarked, chargeID)

	return nil
}

func (store *stubOutboxStore) ListPendingCheckouts(_ context.Context, _ time.Duration, _ int) ([]Charge, error) {
	return store.pendingCheckouts, nil
}

func (store *stubOutboxStore) ListPendingResults(_ context.Context, _ time.Duration, _ int) ([]Charge, error) {
	return store.pendingResults, nil
}

// stubPublisher faz o papel de assinador e de publicador confirmado.
type stubPublisher struct {
	err       error
	calls     int
	signs     int
	eventType string
	payload   map[string]any
	published []events.EventEnvelope
}

func (publisher *stubPublisher) Sign(eventType string, payload any) (events.EventEnvelope, error) {
	publisher.signs++
	publisher.eventType = eventType

	encoded, _ := json.Marshal(payload)
	publisher.payload = make(map[string]any)
	_ = json.Unmarshal(encoded, &publisher.payload)

	return events.EventEnvelope{
		EventID:   fmt.Sprintf("evento-pagamento-%d", publisher.signs),
		EventType: eventType,
		Producer:  events.ProducerPayment,
		Payload:   encoded,
		Signature: "assinatura",
	}, nil
}

func (publisher *stubPublisher) Publish(_ context.Context, envelope events.EventEnvelope) error {
	publisher.calls++

	if publisher.err != nil {
		return publisher.err
	}

	publisher.published = append(publisher.published, envelope)

	return nil
}

func storedPaymentEnvelope(t *testing.T, eventID string, eventType string) string {
	t.Helper()

	encoded, err := json.Marshal(events.EventEnvelope{
		EventID:   eventID,
		EventType: eventType,
		Producer:  events.ProducerPayment,
		Payload:   json.RawMessage(`{"order_id":"PED-001"}`),
		Signature: "assinatura",
	})
	if err != nil {
		t.Fatalf("erro ao montar envelope: %v", err)
	}

	return string(encoded)
}

func TestPublishPendingSendsCheckoutsBeforeResults(t *testing.T) {
	store := &stubOutboxStore{
		pendingCheckouts: []Charge{{
			ID:               "c1",
			OrderID:          "PED-001",
			CheckoutEnvelope: storedPaymentEnvelope(t, "checkout-1", events.PaymentCheckoutAvailable),
		}},
		pendingResults: []Charge{{
			ID:             "c2",
			OrderID:        "PED-002",
			Status:         ChargeApproved,
			ResultEnvelope: storedPaymentEnvelope(t, "resultado-2", events.PaymentApproved),
		}},
	}
	publisher := &stubPublisher{}

	sent, err := NewOutbox(store, publisher).PublishPending(context.Background())
	if err != nil || sent != 2 {
		t.Fatalf("esperava 2 reenvios: %d %v", sent, err)
	}

	if publisher.signs != 0 ||
		publisher.published[0].EventID != "checkout-1" ||
		publisher.published[1].EventID != "resultado-2" {
		t.Fatalf("ordem ou envelopes inesperados: %+v", publisher.published)
	}

	if len(store.checkoutsMarked) != 1 || len(store.resultsMarked) != 1 {
		t.Fatalf("reenvios não marcados: %v %v", store.checkoutsMarked, store.resultsMarked)
	}
}

func TestPublishPendingSignsMissingEnvelopeOnce(t *testing.T) {
	store := &stubOutboxStore{
		pendingResults: []Charge{{
			ID:          "c1",
			OrderID:     "PED-001",
			CustomerID:  "cliente",
			Amount:      59.80,
			Currency:    "BRL",
			Status:      ChargeRefused,
			Reason:      RefusedReason,
			ProcessedAt: time.Date(2026, 10, 1, 18, 10, 0, 0, time.UTC),
		}},
	}
	publisher := &stubPublisher{}

	if _, err := NewOutbox(store, publisher).PublishPending(context.Background()); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if publisher.signs != 1 || publisher.eventType != events.PaymentRefused ||
		publisher.payload["reason"] != RefusedReason || len(store.resultEnvelopes) != 1 {
		t.Fatalf("resultado sem envelope não foi assinado e gravado: %s %+v", publisher.eventType, publisher.payload)
	}
}

func TestPublishPendingStopsWhenBrokerFails(t *testing.T) {
	store := &stubOutboxStore{
		pendingCheckouts: []Charge{{
			ID:               "c1",
			CheckoutEnvelope: storedPaymentEnvelope(t, "checkout-1", events.PaymentCheckoutAvailable),
		}},
	}
	publisher := &stubPublisher{err: errors.New("broker fora do ar")}

	sent, err := NewOutbox(store, publisher).PublishPending(context.Background())
	if err == nil || sent != 0 || len(store.checkoutsMarked) != 0 {
		t.Fatalf("esperava parar sem marcar: %d %v", sent, err)
	}
}
