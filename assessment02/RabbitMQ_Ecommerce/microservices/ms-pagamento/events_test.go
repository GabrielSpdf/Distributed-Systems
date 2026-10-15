package mspagamento

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

type stubChargeStore struct {
	existing      *Charge
	findErr       error
	saveCalls     int
	saved         Charge
	markedChecked []string
}

func (store *stubChargeStore) FindChargeByOrder(_ context.Context, _ string) (Charge, bool, error) {
	if store.existing == nil {
		return Charge{}, false, store.findErr
	}

	return *store.existing, true, store.findErr
}

func (store *stubChargeStore) SaveCharge(_ context.Context, charge Charge) (Charge, error) {
	store.saveCalls++
	store.saved = charge

	return charge, nil
}

func (store *stubChargeStore) MarkCheckoutPublished(_ context.Context, chargeID string) error {
	store.markedChecked = append(store.markedChecked, chargeID)

	return nil
}

type stubChargeCreator struct {
	calls  int
	amount float64
	err    error
}

func (creator *stubChargeCreator) CreateCharge(
	_ context.Context,
	orderID string,
	amount float64,
	_ string,
) (MockCharge, error) {
	creator.calls++
	creator.amount = amount

	return MockCharge{
		ChargeID:    "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
		OrderID:     orderID,
		Status:      ChargePending,
		CheckoutURL: "http://localhost:8083/checkout/d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
		ExpiresAt:   time.Date(2026, 10, 1, 18, 31, 0, 0, time.UTC),
	}, creator.err
}

type stubPublisher struct {
	err       error
	calls     int
	eventType string
	payload   map[string]any
}

func (publisher *stubPublisher) Publish(eventType string, payload any) error {
	publisher.calls++
	publisher.eventType = eventType

	encoded, _ := json.Marshal(payload)
	publisher.payload = make(map[string]any)
	_ = json.Unmarshal(encoded, &publisher.payload)

	return publisher.err
}

const stockConfirmedJSON = `{
	"order_id": "PED-001",
	"customer_id": "083a7808-462b-4df3-a755-3130bc571351",
	"total": 59.80,
	"currency": "BRL",
	"reserved_at": "2026-10-01T18:01:00Z"
}`

func stockConfirmed(payload string) events.EventEnvelope {
	return events.EventEnvelope{
		EventID:   "evento-de-teste",
		EventType: events.OrderStockConfirmed,
		Payload:   json.RawMessage(payload),
	}
}

func TestStockConfirmedCreatesChargeAndPublishesCheckout(t *testing.T) {
	store := &stubChargeStore{}
	creator := &stubChargeCreator{}
	publisher := &stubPublisher{}

	if err := NewEventHandler(store, creator, publisher).Handle(stockConfirmed(stockConfirmedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if creator.calls != 1 || creator.amount != 59.80 {
		t.Fatalf("cobrança inesperada no Mock: %d %v", creator.calls, creator.amount)
	}

	if store.saveCalls != 1 || store.saved.ID != "d67af0cb-b7e8-44c7-a035-22c349c2cbc3" ||
		store.saved.CustomerID != "083a7808-462b-4df3-a755-3130bc571351" ||
		store.saved.Status != ChargePending {
		t.Fatalf("cobrança gravada inesperada: %+v", store.saved)
	}

	if publisher.calls != 1 || publisher.eventType != events.PaymentCheckoutAvailable {
		t.Fatalf("publicação inesperada: %d %s", publisher.calls, publisher.eventType)
	}

	expected := map[string]any{
		"order_id":     "PED-001",
		"customer_id":  "083a7808-462b-4df3-a755-3130bc571351",
		"charge_id":    "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
		"checkout_url": "http://localhost:8083/checkout/d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
		"expires_at":   "2026-10-01T18:31:00Z",
	}

	for key, value := range expected {
		if publisher.payload[key] != value {
			t.Fatalf("campo %s inesperado: %v", key, publisher.payload[key])
		}
	}

	if len(store.markedChecked) != 1 {
		t.Fatal("publicação do checkout não foi marcada")
	}
}

func TestStockConfirmedWithPublishedCheckoutDoesNothing(t *testing.T) {
	store := &stubChargeStore{existing: &Charge{
		ID: "c1", OrderID: "PED-001", Status: ChargePending, CheckoutPublished: true,
	}}
	creator := &stubChargeCreator{}
	publisher := &stubPublisher{}

	if err := NewEventHandler(store, creator, publisher).Handle(stockConfirmed(stockConfirmedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if creator.calls != 0 || publisher.calls != 0 {
		t.Fatalf("evento repetido gerou efeitos: mock=%d publicações=%d", creator.calls, publisher.calls)
	}
}

func TestStockConfirmedRepublishesUnpublishedCheckout(t *testing.T) {
	store := &stubChargeStore{existing: &Charge{
		ID:          "c1",
		OrderID:     "PED-001",
		CustomerID:  "083a7808-462b-4df3-a755-3130bc571351",
		Status:      ChargePending,
		CheckoutURL: "http://localhost:8083/checkout/c1",
		ExpiresAt:   time.Date(2026, 10, 1, 18, 31, 0, 0, time.UTC),
	}}
	creator := &stubChargeCreator{}
	publisher := &stubPublisher{}

	if err := NewEventHandler(store, creator, publisher).Handle(stockConfirmed(stockConfirmedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if creator.calls != 0 {
		t.Fatal("cobrança existente foi recriada no Mock")
	}

	if publisher.calls != 1 || publisher.payload["charge_id"] != "c1" {
		t.Fatalf("checkout pendente não publicado: %+v", publisher.payload)
	}
}

func TestStockConfirmedRejectsInvalidPayload(t *testing.T) {
	cases := map[string]string{
		"JSON inválido":   `{"order_id":`,
		"sem pedido":      `{"customer_id":"c","total":10,"currency":"BRL"}`,
		"sem cliente":     `{"order_id":"PED-001","total":10,"currency":"BRL"}`,
		"total zero":      `{"order_id":"PED-001","customer_id":"c","total":0,"currency":"BRL"}`,
		"moeda diferente": `{"order_id":"PED-001","customer_id":"c","total":10,"currency":"USD"}`,
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			creator := &stubChargeCreator{}
			publisher := &stubPublisher{}

			err := NewEventHandler(&stubChargeStore{}, creator, publisher).Handle(stockConfirmed(payload))
			if err == nil {
				t.Fatal("esperava erro")
			}

			if creator.calls != 0 || publisher.calls != 0 {
				t.Fatal("payload inválido gerou cobrança ou publicação")
			}
		})
	}
}

func TestStockConfirmedMockFailurePublishesNothing(t *testing.T) {
	store := &stubChargeStore{}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, &stubChargeCreator{err: errors.New("mock fora do ar")}, publisher).
		Handle(stockConfirmed(stockConfirmedJSON))
	if err == nil {
		t.Fatal("esperava erro")
	}

	if store.saveCalls != 0 || publisher.calls != 0 {
		t.Fatal("falha no Mock gerou gravação ou publicação")
	}
}

func TestStockConfirmedPublishFailureLeavesCheckoutUnmarked(t *testing.T) {
	store := &stubChargeStore{}
	publisher := &stubPublisher{err: errors.New("broker fora do ar")}

	err := NewEventHandler(store, &stubChargeCreator{}, publisher).Handle(stockConfirmed(stockConfirmedJSON))
	if err == nil {
		t.Fatal("esperava erro")
	}

	if len(store.markedChecked) != 0 {
		t.Fatal("checkout marcado como publicado sem publicação")
	}
}

func TestUnexpectedEventIsRejected(t *testing.T) {
	err := NewEventHandler(&stubChargeStore{}, &stubChargeCreator{}, &stubPublisher{}).Handle(
		events.EventEnvelope{EventType: events.OrderCreated, Payload: json.RawMessage(`{}`)},
	)
	if err == nil {
		t.Fatal("esperava erro para evento inesperado")
	}
}
