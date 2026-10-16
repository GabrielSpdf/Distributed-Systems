package mspagamento

import (
	"context"
	"errors"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

type stubExpiryStore struct {
	stubOutboxStore
	due    []Charge
	err    error
	grace  time.Duration
	called int
}

func (store *stubExpiryStore) ExpireNextCharge(_ context.Context, grace time.Duration) (Charge, bool, error) {
	store.called++
	store.grace = grace

	if store.err != nil {
		return Charge{}, false, store.err
	}

	if len(store.due) == 0 {
		return Charge{}, false, nil
	}

	next := store.due[0]
	store.due = store.due[1:]

	return next, true, nil
}

func expiredCharge(id string) Charge {
	return Charge{
		ID:                id,
		OrderID:           "PED-" + id,
		CustomerID:        "083a7808-462b-4df3-a755-3130bc571351",
		Amount:            59.80,
		Currency:          "BRL",
		Status:            ChargeExpired,
		Reason:            events.ReasonCheckoutExpired,
		ProcessedAt:       time.Date(2026, 10, 1, 18, 31, 5, 0, time.UTC),
		CheckoutPublished: true,
	}
}

func TestExpireDuePublishesCheckoutExpiredRefusal(t *testing.T) {
	store := &stubExpiryStore{due: []Charge{expiredCharge("c1"), expiredCharge("c2")}}
	publisher := &stubPublisher{}

	expired, err := ExpireDue(context.Background(), store, NewOutbox(store, publisher), 5*time.Second)
	if err != nil || expired != 2 {
		t.Fatalf("esperava 2 expirações: %d %v", expired, err)
	}

	if store.grace != 5*time.Second {
		t.Fatalf("margem inesperada: %s", store.grace)
	}

	if publisher.eventType != events.PaymentRefused ||
		publisher.payload["reason"] != events.ReasonCheckoutExpired ||
		len(publisher.published) != 2 || len(store.resultsMarked) != 2 {
		t.Fatalf("expiração inesperada: %s %+v publicados=%d", publisher.eventType, publisher.payload, len(publisher.published))
	}
}

func TestExpireDueReportsStoreFailure(t *testing.T) {
	store := &stubExpiryStore{err: errors.New("banco fora do ar")}

	if _, err := ExpireDue(context.Background(), store, NewOutbox(store, &stubPublisher{}), time.Second); err == nil {
		t.Fatal("esperava erro do banco")
	}
}
