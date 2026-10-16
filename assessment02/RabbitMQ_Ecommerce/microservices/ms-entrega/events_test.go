package msentrega

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

type stubShipmentStore struct {
	existing    *Shipment
	createErr   error
	created     []Shipment
	marked      []string
	pending     []Shipment
	pendingErr  error
	findOrderID string
}

func (store *stubShipmentStore) FindShipment(_ context.Context, orderID string) (Shipment, bool, error) {
	store.findOrderID = orderID

	if store.existing == nil {
		return Shipment{}, false, nil
	}

	return *store.existing, true, nil
}

func (store *stubShipmentStore) CreateShipment(_ context.Context, shipment Shipment) (Shipment, error) {
	if store.createErr != nil {
		return Shipment{}, store.createErr
	}

	store.created = append(store.created, shipment)

	return shipment, nil
}

func (store *stubShipmentStore) MarkPublished(_ context.Context, orderID string) error {
	store.marked = append(store.marked, orderID)

	return nil
}

func (store *stubShipmentStore) ListPendingShipments(_ context.Context, _ time.Duration, _ int) ([]Shipment, error) {
	return store.pending, store.pendingErr
}

type stubSigner struct {
	calls     int
	eventType string
	payload   events.WebOrderShippedPayload
}

func (signer *stubSigner) Sign(eventType string, payload any) (events.EventEnvelope, error) {
	signer.calls++
	signer.eventType = eventType
	signer.payload = payload.(events.WebOrderShippedPayload)

	encoded, _ := json.Marshal(payload)

	return events.EventEnvelope{
		EventID:   "evento-enviado-1",
		EventType: eventType,
		Producer:  events.ProducerDelivery,
		Timestamp: fixedNow,
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

var fixedNow = time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC)

const approvedJSON = `{
	"order_id": "PED-001",
	"customer_id": "083a7808-462b-4df3-a755-3130bc571351",
	"charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
	"amount": 59.80,
	"currency": "BRL",
	"processed_at": "2026-10-15T17:59:00Z"
}`

func approved(payload string) events.EventEnvelope {
	return events.EventEnvelope{
		EventID:   "evento-aprovado-1",
		EventType: events.PaymentApproved,
		Payload:   json.RawMessage(payload),
	}
}

func newTestHandler(store *stubShipmentStore, signer *stubSigner, publisher *stubEnvelopePublisher) *EventHandler {
	handler := NewEventHandler(store, signer, publisher)
	handler.now = func() time.Time { return fixedNow }

	return handler
}

func storedEnvelope(t *testing.T, eventID string) string {
	t.Helper()

	encoded, err := json.Marshal(events.EventEnvelope{
		EventID:   eventID,
		EventType: events.OrderShipped,
		Producer:  events.ProducerDelivery,
		Timestamp: fixedNow,
		Payload:   json.RawMessage(`{"order_id":"PED-001"}`),
		Signature: "assinatura",
	})
	if err != nil {
		t.Fatalf("erro ao montar envelope: %v", err)
	}

	return string(encoded)
}

func TestPaymentApprovedCreatesShipmentAndPublishes(t *testing.T) {
	store := &stubShipmentStore{}
	signer := &stubSigner{}
	publisher := &stubEnvelopePublisher{}

	if err := newTestHandler(store, signer, publisher).Handle(approved(approvedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if len(store.created) != 1 {
		t.Fatalf("esperava 1 entrega criada, houve %d", len(store.created))
	}

	shipment := store.created[0]

	if shipment.OrderID != "PED-001" ||
		shipment.CustomerID != "083a7808-462b-4df3-a755-3130bc571351" ||
		shipment.ChargeID != "d67af0cb-b7e8-44c7-a035-22c349c2cbc3" ||
		!shipment.ShippedAt.Equal(fixedNow) {
		t.Fatalf("entrega inesperada: %+v", shipment)
	}

	if !regexp.MustCompile(`^NF-20261015-\d{6}$`).MatchString(shipment.InvoiceID) {
		t.Fatalf("nota fiscal inesperada: %s", shipment.InvoiceID)
	}

	if !regexp.MustCompile(`^BR\d{9}BR$`).MatchString(shipment.TrackingCode) {
		t.Fatalf("rastreio inesperado: %s", shipment.TrackingCode)
	}

	if signer.eventType != events.OrderShipped ||
		signer.payload.OrderID != "PED-001" ||
		signer.payload.CustomerID != shipment.CustomerID ||
		signer.payload.InvoiceID != shipment.InvoiceID ||
		signer.payload.TrackingCode != shipment.TrackingCode ||
		!signer.payload.ShippedAt.Equal(fixedNow) {
		t.Fatalf("pedido.enviado inesperado: %s %+v", signer.eventType, signer.payload)
	}

	if !strings.Contains(shipment.Envelope, `"event_id":"evento-enviado-1"`) {
		t.Fatalf("envelope não foi gravado antes da publicação: %s", shipment.Envelope)
	}

	if len(publisher.published) != 1 || publisher.published[0].EventID != "evento-enviado-1" {
		t.Fatalf("publicação inesperada: %+v", publisher.published)
	}

	if len(store.marked) != 1 || store.marked[0] != "PED-001" {
		t.Fatalf("publicação não foi marcada: %v", store.marked)
	}
}

func TestRepeatedApprovalWithPublishedShipmentDoesNothing(t *testing.T) {
	store := &stubShipmentStore{existing: &Shipment{
		OrderID:    "PED-001",
		CustomerID: "083a7808-462b-4df3-a755-3130bc571351",
		Envelope:   storedEnvelope(t, "evento-original"),
		Published:  true,
	}}
	signer := &stubSigner{}
	publisher := &stubEnvelopePublisher{}

	if err := newTestHandler(store, signer, publisher).Handle(approved(approvedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if signer.calls != 0 || len(store.created) != 0 || len(publisher.published) != 0 {
		t.Fatalf("aprovação repetida gerou efeitos: assinaturas=%d criadas=%d publicações=%d",
			signer.calls, len(store.created), len(publisher.published))
	}
}

func TestRepeatedApprovalRepublishesTheSameStoredEvent(t *testing.T) {
	store := &stubShipmentStore{existing: &Shipment{
		OrderID:    "PED-001",
		CustomerID: "083a7808-462b-4df3-a755-3130bc571351",
		Envelope:   storedEnvelope(t, "evento-original"),
	}}
	signer := &stubSigner{}
	publisher := &stubEnvelopePublisher{}

	if err := newTestHandler(store, signer, publisher).Handle(approved(approvedJSON)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	if signer.calls != 0 || len(store.created) != 0 {
		t.Fatal("entrega existente foi recriada")
	}

	if len(publisher.published) != 1 || publisher.published[0].EventID != "evento-original" {
		t.Fatalf("esperava reenviar o evento original: %+v", publisher.published)
	}
}

func TestApprovalForAnotherCustomerIsRejected(t *testing.T) {
	store := &stubShipmentStore{existing: &Shipment{
		OrderID:    "PED-001",
		CustomerID: "outro-cliente",
		Envelope:   storedEnvelope(t, "evento-original"),
	}}
	publisher := &stubEnvelopePublisher{}

	if err := newTestHandler(store, &stubSigner{}, publisher).Handle(approved(approvedJSON)); err == nil {
		t.Fatal("esperava erro para cliente divergente")
	}

	if len(publisher.published) != 0 {
		t.Fatal("entrega de outro cliente foi publicada")
	}
}

func TestInvalidApprovalIsRejected(t *testing.T) {
	replace := func(old, new string) string { return strings.Replace(approvedJSON, old, new, 1) }

	cases := map[string]string{
		"JSON inválido":   `{"order_id":`,
		"sem pedido":      replace(`"PED-001"`, `""`),
		"sem cliente":     replace(`"083a7808-462b-4df3-a755-3130bc571351"`, `""`),
		"sem cobrança":    replace(`"d67af0cb-b7e8-44c7-a035-22c349c2cbc3"`, `""`),
		"valor zero":      replace(`59.80`, `0`),
		"moeda diferente": replace(`"BRL"`, `"USD"`),
		"sem data":        replace(`"2026-10-15T17:59:00Z"`, `"0001-01-01T00:00:00Z"`),
	}

	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			store := &stubShipmentStore{}
			publisher := &stubEnvelopePublisher{}

			if err := newTestHandler(store, &stubSigner{}, publisher).Handle(approved(payload)); err == nil {
				t.Fatal("esperava erro")
			}

			if len(store.created) != 0 || len(publisher.published) != 0 {
				t.Fatal("aprovação inválida gerou entrega")
			}
		})
	}
}

func TestPublishFailureKeepsShipmentPending(t *testing.T) {
	store := &stubShipmentStore{}
	publisher := &stubEnvelopePublisher{err: errors.New("broker fora do ar")}

	// A entrega já está gravada com o envelope: confirmar a mensagem é seguro,
	// e a rotina de pendências reenvia depois.
	if err := newTestHandler(store, &stubSigner{}, publisher).Handle(approved(approvedJSON)); err != nil {
		t.Fatalf("falha de publicação com entrega gravada não deve rejeitar a mensagem: %v", err)
	}

	if len(store.created) != 1 || len(store.marked) != 0 {
		t.Fatalf("esperava entrega gravada e pendente: criadas=%d marcadas=%d", len(store.created), len(store.marked))
	}
}

func TestStoreFailureIsReported(t *testing.T) {
	store := &stubShipmentStore{createErr: errors.New("banco fora do ar")}
	publisher := &stubEnvelopePublisher{}

	if err := newTestHandler(store, &stubSigner{}, publisher).Handle(approved(approvedJSON)); err == nil {
		t.Fatal("esperava erro")
	}

	if len(publisher.published) != 0 {
		t.Fatal("houve publicação sem entrega gravada")
	}
}

func TestUnexpectedEventIsRejected(t *testing.T) {
	err := newTestHandler(&stubShipmentStore{}, &stubSigner{}, &stubEnvelopePublisher{}).Handle(
		events.EventEnvelope{EventType: events.PaymentRefused, Payload: json.RawMessage(approvedJSON)},
	)
	if err == nil {
		t.Fatal("esperava erro para evento inesperado")
	}
}

func TestPublishPendingResendsStoredEvents(t *testing.T) {
	store := &stubShipmentStore{pending: []Shipment{
		{OrderID: "PED-001", Envelope: storedEnvelope(t, "evento-1")},
		{OrderID: "PED-002", Envelope: storedEnvelope(t, "evento-2")},
	}}
	publisher := &stubEnvelopePublisher{}

	sent, err := newTestHandler(store, &stubSigner{}, publisher).PublishPending(context.Background())
	if err != nil || sent != 2 {
		t.Fatalf("esperava 2 reenvios, houve %d: %v", sent, err)
	}

	if publisher.published[0].EventID != "evento-1" || publisher.published[1].EventID != "evento-2" {
		t.Fatalf("eventos reenviados inesperados: %+v", publisher.published)
	}

	if len(store.marked) != 2 {
		t.Fatalf("reenvios não marcados: %v", store.marked)
	}
}

func TestPublishPendingStopsWhenBrokerFails(t *testing.T) {
	store := &stubShipmentStore{pending: []Shipment{
		{OrderID: "PED-001", Envelope: storedEnvelope(t, "evento-1")},
		{OrderID: "PED-002", Envelope: storedEnvelope(t, "evento-2")},
	}}
	publisher := &stubEnvelopePublisher{err: errors.New("broker fora do ar")}

	sent, err := newTestHandler(store, &stubSigner{}, publisher).PublishPending(context.Background())
	if err == nil || sent != 0 || len(store.marked) != 0 {
		t.Fatalf("esperava parar sem marcar: enviados=%d marcados=%v err=%v", sent, store.marked, err)
	}
}

func TestErrorKindsForTheReliableConsumer(t *testing.T) {
	invalid := newTestHandler(&stubShipmentStore{}, &stubSigner{}, &stubEnvelopePublisher{}).
		Handle(approved(`{"order_id":"PED-001"}`))
	if !rabbitmq.IsPermanent(invalid) {
		t.Fatalf("aprovação inválida deveria ser permanente: %v", invalid)
	}

	otherCustomer := newTestHandler(&stubShipmentStore{existing: &Shipment{
		OrderID:    "PED-001",
		CustomerID: "outro-cliente",
		Envelope:   storedEnvelope(t, "evento-original"),
	}}, &stubSigner{}, &stubEnvelopePublisher{}).Handle(approved(approvedJSON))
	if !rabbitmq.IsPermanent(otherCustomer) {
		t.Fatalf("cliente divergente deveria ser permanente: %v", otherCustomer)
	}

	unexpected := newTestHandler(&stubShipmentStore{}, &stubSigner{}, &stubEnvelopePublisher{}).Handle(
		events.EventEnvelope{EventType: events.PaymentRefused, Payload: json.RawMessage(approvedJSON)},
	)
	if !rabbitmq.IsPermanent(unexpected) {
		t.Fatalf("evento inesperado deveria ser permanente: %v", unexpected)
	}

	databaseDown := newTestHandler(&stubShipmentStore{createErr: errors.New("banco fora do ar")},
		&stubSigner{}, &stubEnvelopePublisher{}).Handle(approved(approvedJSON))
	if databaseDown == nil || rabbitmq.IsPermanent(databaseDown) {
		t.Fatalf("banco fora do ar deveria ser temporário: %v", databaseDown)
	}
}
