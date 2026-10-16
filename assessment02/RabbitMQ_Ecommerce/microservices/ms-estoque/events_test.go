package msestoque

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
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

	savedEnvelopes []string
	saveErr        error
	marked         []string
	pending        []PendingResult
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

func (store *stubReservationStore) SaveResultEnvelope(
	_ context.Context,
	_ string,
	envelope string,
) (string, error) {
	if store.saveErr != nil {
		return "", store.saveErr
	}

	store.savedEnvelopes = append(store.savedEnvelopes, envelope)

	return envelope, nil
}

func (store *stubReservationStore) MarkResultPublished(_ context.Context, orderID string) error {
	store.marked = append(store.marked, orderID)

	return nil
}

func (store *stubReservationStore) ListPendingResults(
	_ context.Context,
	_ time.Duration,
	_ int,
) ([]PendingResult, error) {
	return store.pending, nil
}

type stubSigner struct {
	calls     int
	eventType string
	payload   map[string]any
}

func (signer *stubSigner) Sign(eventType string, payload any) (events.EventEnvelope, error) {
	signer.calls++
	signer.eventType = eventType

	encoded, _ := json.Marshal(payload)
	signer.payload = make(map[string]any)
	_ = json.Unmarshal(encoded, &signer.payload)

	return events.EventEnvelope{
		EventID:   "evento-estoque-1",
		EventType: eventType,
		Producer:  events.ProducerStock,
		Payload:   encoded,
		Signature: "assinatura",
	}, nil
}

type stubEnvelopePublisher struct {
	err       error
	published []events.EventEnvelope
}

func (publisher *stubEnvelopePublisher) Publish(_ context.Context, envelope events.EventEnvelope) error {
	if publisher.err != nil {
		return publisher.err
	}

	publisher.published = append(publisher.published, envelope)

	return nil
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

func storedEnvelope(t *testing.T, eventID string) string {
	t.Helper()

	encoded, err := json.Marshal(events.EventEnvelope{
		EventID:   eventID,
		EventType: events.OrderStockConfirmed,
		Producer:  events.ProducerStock,
		Payload:   json.RawMessage(`{"order_id":"PED-001"}`),
		Signature: "assinatura",
	})
	if err != nil {
		t.Fatalf("erro ao montar envelope: %v", err)
	}

	return string(encoded)
}

type handlerParts struct {
	store     *stubReservationStore
	signer    *stubSigner
	publisher *stubEnvelopePublisher
	handler   *EventHandler
}

func newParts(result ReservationResult) handlerParts {
	parts := handlerParts{
		store:     &stubReservationStore{reserveResult: result},
		signer:    &stubSigner{},
		publisher: &stubEnvelopePublisher{},
	}
	parts.handler = NewEventHandler(parts.store, parts.signer, parts.publisher)

	return parts
}

func TestOrderCreatedReservedPublishesStockConfirmed(t *testing.T) {
	reservedAt := time.Date(2026, 10, 1, 18, 1, 0, 0, time.UTC)
	parts := newParts(ReservationResult{Status: ReservationReserved, ReservedAt: reservedAt})

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	expectedItems := []ReservationItem{
		{ProductID: "PROD-001", Quantity: 2},
		{ProductID: "PROD-002", Quantity: 3},
	}

	if parts.store.reservedOrder != "PED-001" || !reflect.DeepEqual(parts.store.reservedItems, expectedItems) {
		t.Fatalf("reserva inesperada: %s %+v", parts.store.reservedOrder, parts.store.reservedItems)
	}

	expectedPayload := map[string]any{
		"order_id":    "PED-001",
		"customer_id": "083a7808-462b-4df3-a755-3130bc571351",
		"total":       74.50,
		"currency":    "BRL",
		"reserved_at": "2026-10-01T18:01:00Z",
	}

	if parts.signer.eventType != events.OrderStockConfirmed || !reflect.DeepEqual(parts.signer.payload, expectedPayload) {
		t.Fatalf("pedido.estoque_ok inesperado: %s %+v", parts.signer.eventType, parts.signer.payload)
	}

	if len(parts.store.savedEnvelopes) != 1 {
		t.Fatal("envelope não foi gravado antes da publicação")
	}

	if len(parts.publisher.published) != 1 || parts.publisher.published[0].EventID != "evento-estoque-1" {
		t.Fatalf("publicação inesperada: %+v", parts.publisher.published)
	}

	if len(parts.store.marked) != 1 || parts.store.marked[0] != "PED-001" {
		t.Fatalf("publicação não marcada: %v", parts.store.marked)
	}
}

func TestOrderCreatedUnavailablePublishesStockUnavailable(t *testing.T) {
	parts := newParts(ReservationResult{
		Status:     ReservationUnavailable,
		Shortfalls: []Shortfall{{ProductID: "PROD-001", RequestedQuantity: 2, AvailableQuantity: 1}},
	})

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
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

	if parts.signer.eventType != events.StockUnavailable || !reflect.DeepEqual(parts.signer.payload, expectedPayload) {
		t.Fatalf("estoque.indisponivel inesperado: %s %+v", parts.signer.eventType, parts.signer.payload)
	}

	if len(parts.publisher.published) != 1 {
		t.Fatal("estoque.indisponivel não publicado")
	}
}

func TestOrderCreatedUnavailableWithoutShortfallsPublishesNothing(t *testing.T) {
	parts := newParts(ReservationResult{Status: ReservationUnavailable, Shortfalls: []Shortfall{}})

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if parts.signer.calls != 0 || len(parts.publisher.published) != 0 {
		t.Fatal("estoque.indisponivel sem itens não deve ser publicado")
	}
}

func TestOrderCreatedAfterCancellationPublishesNothing(t *testing.T) {
	parts := newParts(ReservationResult{Status: ReservationReleased})

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if parts.signer.calls != 0 || len(parts.publisher.published) != 0 {
		t.Fatal("pedido cancelado gerou publicação")
	}
}

func TestRepeatedOrderResendsTheStoredEvent(t *testing.T) {
	parts := newParts(ReservationResult{
		Status:   ReservationReserved,
		Envelope: storedEnvelope(t, "evento-original"),
	})

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if parts.signer.calls != 0 || len(parts.store.savedEnvelopes) != 0 {
		t.Fatal("pedido repetido gerou um novo envelope")
	}

	if len(parts.publisher.published) != 1 || parts.publisher.published[0].EventID != "evento-original" {
		t.Fatalf("esperava reenviar o evento original: %+v", parts.publisher.published)
	}
}

func TestRepeatedOrderAlreadyPublishedDoesNothing(t *testing.T) {
	parts := newParts(ReservationResult{
		Status:    ReservationReserved,
		Envelope:  storedEnvelope(t, "evento-original"),
		Published: true,
	})

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if parts.signer.calls != 0 || len(parts.publisher.published) != 0 {
		t.Fatal("resultado já publicado foi reenviado")
	}
}

func TestOrderCreatedDefaultsCurrencyToBRL(t *testing.T) {
	parts := newParts(ReservationResult{Status: ReservationReserved})

	err := parts.handler.Handle(envelopeOf(
		events.OrderCreated,
		`{"order_id":"PED-002","customer_id":"cliente","items":[{"product_id":"PROD-001","quantity":1}],"total":29.90}`,
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if parts.signer.payload["currency"] != "BRL" {
		t.Fatalf("moeda inesperada: %v", parts.signer.payload["currency"])
	}
}

func TestOrderCreatedRejectsInvalidPayloadPermanently(t *testing.T) {
	cases := map[string]string{
		"JSON inválido": `{"order_id":`,
		"sem pedido":    `{"customer_id":"cliente","items":[{"product_id":"PROD-001","quantity":1}]}`,
		"sem cliente":   `{"order_id":"PED-001","items":[{"product_id":"PROD-001","quantity":1}]}`,
		"sem itens":     `{"order_id":"PED-001","customer_id":"cliente","items":[]}`,
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			parts := newParts(ReservationResult{})

			err := parts.handler.Handle(envelopeOf(events.OrderCreated, payload))
			if !rabbitmq.IsPermanent(err) {
				t.Fatalf("esperava erro permanente, recebeu %v", err)
			}

			if parts.store.reserveCalls != 0 || len(parts.publisher.published) != 0 {
				t.Fatal("payload inválido chegou ao banco ou foi publicado")
			}
		})
	}
}

func TestInvalidOrderFromStoreIsPermanent(t *testing.T) {
	parts := newParts(ReservationResult{})
	parts.store.reserveErr = ErrInvalidOrder

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); !rabbitmq.IsPermanent(err) {
		t.Fatalf("pedido inválido deveria ser permanente: %v", err)
	}
}

func TestStoreFailureIsTemporaryAndPublishesNothing(t *testing.T) {
	parts := newParts(ReservationResult{})
	parts.store.reserveErr = errors.New("banco fora do ar")

	err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON))
	if err == nil || rabbitmq.IsPermanent(err) {
		t.Fatalf("falha no banco deveria ser temporária: %v", err)
	}

	if len(parts.publisher.published) != 0 {
		t.Fatal("houve publicação sem persistência")
	}
}

func TestPublishFailureKeepsResultPending(t *testing.T) {
	parts := newParts(ReservationResult{Status: ReservationReserved})
	parts.publisher.err = errors.New("broker fora do ar")

	if err := parts.handler.Handle(envelopeOf(events.OrderCreated, orderCreatedJSON)); err != nil {
		t.Fatalf("com o envelope gravado, a falha de publicação não deve rejeitar: %v", err)
	}

	if len(parts.store.savedEnvelopes) != 1 || len(parts.store.marked) != 0 {
		t.Fatal("esperava envelope gravado e resultado pendente")
	}
}

func TestOrderDeletedReleasesStock(t *testing.T) {
	parts := newParts(ReservationResult{})

	err := parts.handler.Handle(envelopeOf(
		events.OrderDeleted,
		`{"order_id":"PED-001","customer_id":"cliente","reason":"PAYMENT_REFUSED","cancelled_at":"2026-10-01T18:11:00Z"}`,
	))
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if parts.store.releaseCalls != 1 || parts.store.releasedOrder != "PED-001" || len(parts.publisher.published) != 0 {
		t.Fatalf("liberação inesperada: %d %s", parts.store.releaseCalls, parts.store.releasedOrder)
	}
}

func TestOrderDeletedRejectsMissingOrderPermanently(t *testing.T) {
	parts := newParts(ReservationResult{})

	err := parts.handler.Handle(envelopeOf(events.OrderDeleted, `{"reason":"CUSTOMER_CANCELLED"}`))
	if !rabbitmq.IsPermanent(err) || parts.store.releaseCalls != 0 {
		t.Fatalf("esperava erro permanente sem liberação: %v", err)
	}
}

func TestOrderDeletedStoreFailureIsTemporary(t *testing.T) {
	parts := newParts(ReservationResult{})
	parts.store.releaseErr = errors.New("banco fora do ar")

	err := parts.handler.Handle(envelopeOf(events.OrderDeleted, `{"order_id":"PED-001"}`))
	if err == nil || rabbitmq.IsPermanent(err) {
		t.Fatalf("falha no banco deveria ser temporária: %v", err)
	}
}

func TestUnexpectedEventIsRejectedPermanently(t *testing.T) {
	parts := newParts(ReservationResult{})

	if err := parts.handler.Handle(envelopeOf(events.PaymentApproved, `{}`)); !rabbitmq.IsPermanent(err) {
		t.Fatalf("evento inesperado deveria ser permanente: %v", err)
	}
}

func TestPublishPendingResendsStoredResults(t *testing.T) {
	parts := newParts(ReservationResult{})
	parts.store.pending = []PendingResult{
		{OrderID: "PED-001", Envelope: storedEnvelope(t, "evento-1")},
		{OrderID: "PED-002", Envelope: storedEnvelope(t, "evento-2")},
	}

	sent, err := parts.handler.PublishPending(context.Background())
	if err != nil || sent != 2 || len(parts.store.marked) != 2 ||
		parts.publisher.published[0].EventID != "evento-1" {
		t.Fatalf("reenvio inesperado: %d %v %+v", sent, err, parts.publisher.published)
	}
}

func TestPublishPendingStopsWhenBrokerFails(t *testing.T) {
	parts := newParts(ReservationResult{})
	parts.store.pending = []PendingResult{{OrderID: "PED-001", Envelope: storedEnvelope(t, "evento-1")}}
	parts.publisher.err = errors.New("broker fora do ar")

	sent, err := parts.handler.PublishPending(context.Background())
	if err == nil || sent != 0 || len(parts.store.marked) != 0 {
		t.Fatalf("esperava parar sem marcar: %d %v", sent, err)
	}
}
