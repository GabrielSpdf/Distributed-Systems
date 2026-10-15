package msestoque

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

type stubReservationStore struct {
	reserveResult ReservationResult
	reserveErr    error
	reserveCalls  int
	reservedOrder string
	reservedItems []ReservationItem

	releaseErr    error
	releaseCalls  int
	releasedOrder string
}

func (store *stubReservationStore) Reserve(
	_ context.Context,
	orderID string,
	items []ReservationItem,
) (ReservationResult, error) {
	store.reserveCalls++
	store.reservedOrder = orderID
	store.reservedItems = items

	return store.reserveResult, store.reserveErr
}

func (store *stubReservationStore) Release(
	_ context.Context,
	orderID string,
) (string, error) {
	store.releaseCalls++
	store.releasedOrder = orderID

	return ReservationReleased, store.releaseErr
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

	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	publisher.payload = make(map[string]any)

	if err := json.Unmarshal(encoded, &publisher.payload); err != nil {
		return err
	}

	return publisher.err
}

const orderCreatedJSON = `{
	"order_id": "PED-001",
	"customer_id": "083a7808-462b-4df3-a755-3130bc571351",
	"items": [
		{"product_id": "PROD-001", "product_name": "Arroz 5 kg", "quantity": 2, "unit_price": 29.90},
		{"product_id": "PROD-002", "product_name": "Detergente", "quantity": 3, "unit_price": 4.90}
	],
	"total": 74.50,
	"currency": "BRL"
}`

func envelopeOf(eventType string, payload string) events.EventEnvelope {
	return events.EventEnvelope{
		EventID:   "evento-de-teste",
		EventType: eventType,
		Payload:   json.RawMessage(payload),
	}
}

func TestOrderCreatedReservedPublishesStockConfirmed(t *testing.T) {
	reservedAt := time.Date(2026, 10, 1, 18, 1, 0, 0, time.UTC)

	store := &stubReservationStore{
		reserveResult: ReservationResult{
			Status:     ReservationReserved,
			ReservedAt: reservedAt,
		},
	}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(
		envelopeOf(events.OrderCreated, orderCreatedJSON),
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if store.reservedOrder != "PED-001" {
		t.Fatalf("pedido inesperado: %s", store.reservedOrder)
	}

	expectedItems := []ReservationItem{
		{ProductID: "PROD-001", Quantity: 2},
		{ProductID: "PROD-002", Quantity: 3},
	}

	if !reflect.DeepEqual(store.reservedItems, expectedItems) {
		t.Fatalf("itens inesperados: %+v", store.reservedItems)
	}

	if publisher.calls != 1 || publisher.eventType != events.OrderStockConfirmed {
		t.Fatalf("publicação inesperada: %d %s", publisher.calls, publisher.eventType)
	}

	expectedPayload := map[string]any{
		"order_id":    "PED-001",
		"customer_id": "083a7808-462b-4df3-a755-3130bc571351",
		"total":       74.50,
		"currency":    "BRL",
		"reserved_at": "2026-10-01T18:01:00Z",
	}

	if !reflect.DeepEqual(publisher.payload, expectedPayload) {
		t.Fatalf("payload inesperado: %+v", publisher.payload)
	}
}

func TestOrderCreatedUnavailablePublishesStockUnavailable(t *testing.T) {
	store := &stubReservationStore{
		reserveResult: ReservationResult{
			Status: ReservationUnavailable,
			Shortfalls: []Shortfall{
				{ProductID: "PROD-001", RequestedQuantity: 2, AvailableQuantity: 1},
			},
		},
	}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(
		envelopeOf(events.OrderCreated, orderCreatedJSON),
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if publisher.calls != 1 || publisher.eventType != events.StockUnavailable {
		t.Fatalf("publicação inesperada: %d %s", publisher.calls, publisher.eventType)
	}

	expectedPayload := map[string]any{
		"order_id":    "PED-001",
		"customer_id": "083a7808-462b-4df3-a755-3130bc571351",
		"unavailable_items": []any{
			map[string]any{
				"product_id":         "PROD-001",
				"requested_quantity": float64(2),
				"available_quantity": float64(1),
			},
		},
		"reason": "Estoque insuficiente",
	}

	if !reflect.DeepEqual(publisher.payload, expectedPayload) {
		t.Fatalf("payload inesperado: %+v", publisher.payload)
	}
}

func TestOrderCreatedUnavailableWithoutShortfallsPublishesNothing(t *testing.T) {
	store := &stubReservationStore{
		reserveResult: ReservationResult{
			Status:     ReservationUnavailable,
			Shortfalls: []Shortfall{},
		},
	}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(
		envelopeOf(events.OrderCreated, orderCreatedJSON),
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if publisher.calls != 0 {
		t.Fatalf("publicou estoque.indisponivel sem itens: %+v", publisher.payload)
	}
}

func TestOrderCreatedAfterCancellationPublishesNothing(t *testing.T) {
	store := &stubReservationStore{
		reserveResult: ReservationResult{Status: ReservationReleased},
	}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(
		envelopeOf(events.OrderCreated, orderCreatedJSON),
	)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if publisher.calls != 0 {
		t.Fatalf("pedido cancelado gerou publicação: %s", publisher.eventType)
	}
}

func TestOrderCreatedDefaultsCurrencyToBRL(t *testing.T) {
	store := &stubReservationStore{
		reserveResult: ReservationResult{Status: ReservationReserved},
	}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(envelopeOf(
		events.OrderCreated,
		`{"order_id":"PED-002","customer_id":"cliente","items":[{"product_id":"PROD-001","quantity":1}],"total":29.90}`,
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if publisher.payload["currency"] != "BRL" {
		t.Fatalf("moeda inesperada: %v", publisher.payload["currency"])
	}
}

func TestOrderCreatedRejectsInvalidPayload(t *testing.T) {
	cases := map[string]string{
		"JSON inválido": `{"order_id":`,
		"sem pedido":    `{"customer_id":"cliente","items":[{"product_id":"PROD-001","quantity":1}]}`,
		"sem cliente":   `{"order_id":"PED-001","items":[{"product_id":"PROD-001","quantity":1}]}`,
		"sem itens":     `{"order_id":"PED-001","customer_id":"cliente","items":[]}`,
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			store := &stubReservationStore{}
			publisher := &stubPublisher{}

			err := NewEventHandler(store, publisher).Handle(
				envelopeOf(events.OrderCreated, payload),
			)
			if err == nil {
				t.Fatal("esperava erro")
			}

			if store.reserveCalls != 0 || publisher.calls != 0 {
				t.Fatal("payload inválido chegou ao banco ou foi publicado")
			}
		})
	}
}

func TestOrderCreatedStoreFailurePublishesNothing(t *testing.T) {
	store := &stubReservationStore{reserveErr: errors.New("banco fora do ar")}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(
		envelopeOf(events.OrderCreated, orderCreatedJSON),
	)
	if err == nil {
		t.Fatal("esperava erro")
	}

	if publisher.calls != 0 {
		t.Fatal("houve publicação sem persistência")
	}
}

func TestOrderCreatedPublishFailureIsReported(t *testing.T) {
	store := &stubReservationStore{
		reserveResult: ReservationResult{Status: ReservationReserved},
	}
	publisher := &stubPublisher{err: errors.New("broker fora do ar")}

	err := NewEventHandler(store, publisher).Handle(
		envelopeOf(events.OrderCreated, orderCreatedJSON),
	)
	if err == nil {
		t.Fatal("esperava erro de publicação")
	}
}

func TestOrderDeletedReleasesStock(t *testing.T) {
	store := &stubReservationStore{}
	publisher := &stubPublisher{}

	err := NewEventHandler(store, publisher).Handle(envelopeOf(
		events.OrderDeleted,
		`{"order_id":"PED-001","customer_id":"cliente","reason":"PAYMENT_REFUSED","cancelled_at":"2026-10-01T18:11:00Z"}`,
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if store.releaseCalls != 1 || store.releasedOrder != "PED-001" {
		t.Fatalf("liberação inesperada: %d %s", store.releaseCalls, store.releasedOrder)
	}

	if publisher.calls != 0 {
		t.Fatal("pedido.excluido não deve gerar publicação")
	}
}

func TestOrderDeletedRejectsMissingOrder(t *testing.T) {
	store := &stubReservationStore{}

	err := NewEventHandler(store, &stubPublisher{}).Handle(
		envelopeOf(events.OrderDeleted, `{"reason":"CUSTOMER_CANCELLED"}`),
	)
	if err == nil {
		t.Fatal("esperava erro")
	}

	if store.releaseCalls != 0 {
		t.Fatal("liberação chamada sem pedido")
	}
}

func TestOrderDeletedStoreFailureIsReported(t *testing.T) {
	store := &stubReservationStore{releaseErr: errors.New("banco fora do ar")}

	err := NewEventHandler(store, &stubPublisher{}).Handle(
		envelopeOf(events.OrderDeleted, `{"order_id":"PED-001"}`),
	)
	if err == nil {
		t.Fatal("esperava erro")
	}
}

func TestUnexpectedEventIsRejected(t *testing.T) {
	err := NewEventHandler(&stubReservationStore{}, &stubPublisher{}).Handle(
		envelopeOf(events.PaymentApproved, `{}`),
	)
	if err == nil {
		t.Fatal("esperava erro para evento inesperado")
	}
}
