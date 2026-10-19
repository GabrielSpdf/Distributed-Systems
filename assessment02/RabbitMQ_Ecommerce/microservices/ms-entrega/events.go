package msentrega

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

const (
	eventHandlerTimeout = 10 * time.Second
	pendingMinimumAge   = 5 * time.Second
	pendingBatchSize    = 20
)

// ShipmentStore is what the handler needs from the database.
type ShipmentStore interface {
	FindShipment(ctx context.Context, orderID string) (Shipment, bool, error)
	CreateShipment(ctx context.Context, shipment Shipment) (Shipment, error)
	MarkPublished(ctx context.Context, orderID string) error
	ListPendingShipments(ctx context.Context, minimumAge time.Duration, limit int) ([]Shipment, error)
}

// EventSigner builds a signed envelope for an outgoing event.
type EventSigner interface {
	Sign(eventType string, payload any) (events.EventEnvelope, error)
}

// EnvelopePublisher sends an already signed envelope and returns only after
// the broker confirms it.
type EnvelopePublisher interface {
	Publish(ctx context.Context, envelope events.EventEnvelope) error
}

type EventHandler struct {
	store     ShipmentStore
	signer    EventSigner
	publisher EnvelopePublisher
	now       func() time.Time
}

func NewEventHandler(
	store ShipmentStore,
	signer EventSigner,
	publisher EnvelopePublisher,
) *EventHandler {
	return &EventHandler{
		store:     store,
		signer:    signer,
		publisher: publisher,
		now:       time.Now,
	}
}

// Handle processes one verified pagamento.aprovado. A nil return lets the
// consumer Ack: that only happens after the shipment and its signed event are
// stored, so a failed publication is retried by PublishPending.
func (handler *EventHandler) Handle(envelope events.EventEnvelope) error {
	if envelope.EventType != events.PaymentApproved {
		return rabbitmq.Permanent(fmt.Errorf("tipo de evento inesperado na Entrega: %s", envelope.EventType))
	}

	ctx, cancel := context.WithTimeout(context.Background(), eventHandlerTimeout)
	defer cancel()

	payment, err := readPaymentApproved(envelope.Payload)
	if err != nil {
		return rabbitmq.Permanent(err)
	}

	shipment, exists, err := handler.store.FindShipment(ctx, payment.OrderID)
	if err != nil {
		return err
	}

	if !exists {
		shipment, err = handler.newShipment(payment)
		if err != nil {
			return err
		}

		shipment, err = handler.store.CreateShipment(ctx, shipment)
		if err != nil {
			if errors.Is(err, ErrInvalidPayment) {
				return rabbitmq.Permanent(err)
			}
			return err
		}

		log.Printf(
			"[SUCESSO] Entrega do pedido %s registrada: nota %s, rastreio %s",
			shipment.OrderID,
			shipment.InvoiceID,
			shipment.TrackingCode,
		)
	}

	snapshot, err := paymentSnapshot(payment)
	if err != nil {
		return rabbitmq.Permanent(fmt.Errorf("%w: %v", ErrInvalidPayment, err))
	}
	if !sameShipmentInput(shipment, Shipment{
		OrderID: payment.OrderID, CustomerID: payment.CustomerID,
		ChargeID: payment.ChargeID, PaymentPayload: snapshot,
	}) {
		return rabbitmq.Permanent(fmt.Errorf(
			"%w: pedido %s tem aprovação divergente ou original ausente",
			ErrInvalidPayment,
			payment.OrderID,
		))
	}

	if shipment.Published {
		log.Printf("[AVISO] Pedido %s já enviado; aprovação repetida ignorada", shipment.OrderID)
		return nil
	}

	if err := handler.publish(ctx, shipment); err != nil {
		log.Printf(
			"[AVISO] pedido.enviado do pedido %s pendente; será reenviado: %v",
			shipment.OrderID,
			err,
		)
	}

	return nil
}

// newShipment simulates the invoice and the dispatch, and signs pedido.enviado
// once. The signed envelope is stored with the shipment.
func (handler *EventHandler) newShipment(
	payment events.WebPaymentResultPayload,
) (Shipment, error) {
	snapshot, err := paymentSnapshot(payment)
	if err != nil {
		return Shipment{}, fmt.Errorf("%w: %v", ErrInvalidPayment, err)
	}
	shippedAt := handler.now().UTC()

	invoiceID, err := newInvoiceID(shippedAt)
	if err != nil {
		return Shipment{}, err
	}

	trackingCode, err := newTrackingCode()
	if err != nil {
		return Shipment{}, err
	}

	envelope, err := handler.signer.Sign(events.OrderShipped, events.WebOrderShippedPayload{
		OrderID:      payment.OrderID,
		CustomerID:   payment.CustomerID,
		InvoiceID:    invoiceID,
		TrackingCode: trackingCode,
		ShippedAt:    shippedAt,
	})
	if err != nil {
		return Shipment{}, fmt.Errorf("erro ao assinar pedido.enviado: %w", err)
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return Shipment{}, fmt.Errorf("erro ao serializar pedido.enviado: %w", err)
	}

	return Shipment{
		OrderID:        payment.OrderID,
		CustomerID:     payment.CustomerID,
		ChargeID:       payment.ChargeID,
		InvoiceID:      invoiceID,
		TrackingCode:   trackingCode,
		ShippedAt:      shippedAt,
		Envelope:       string(encoded),
		PaymentPayload: snapshot,
	}, nil
}

// publish sends the stored envelope (same event_id on every attempt) and marks
// the shipment only after the broker confirmation.
func (handler *EventHandler) publish(ctx context.Context, shipment Shipment) error {
	var envelope events.EventEnvelope

	if err := json.Unmarshal([]byte(shipment.Envelope), &envelope); err != nil {
		return fmt.Errorf("envelope gravado inválido: %w", err)
	}

	if err := handler.publisher.Publish(ctx, envelope); err != nil {
		return err
	}

	if err := handler.store.MarkPublished(ctx, shipment.OrderID); err != nil {
		// O evento saiu; no pior caso será reenviado com o mesmo event_id.
		log.Printf("[ERRO] Marcação da entrega %s: %v", shipment.OrderID, err)
	}

	printBanner("PEDIDO ENVIADO - PEDIDO - %s", shipment.OrderID)
	log.Printf("[SUCESSO] pedido.enviado do pedido %s publicado", shipment.OrderID)

	return nil
}

// PublishPending resends the shipments whose event was not confirmed. It stops
// at the first broker failure and returns how many were sent.
func (handler *EventHandler) PublishPending(ctx context.Context) (int, error) {
	pending, err := handler.store.ListPendingShipments(ctx, pendingMinimumAge, pendingBatchSize)
	if err != nil {
		return 0, err
	}

	sent := 0

	for _, shipment := range pending {
		if err := handler.publish(ctx, shipment); err != nil {
			return sent, fmt.Errorf("reenvio do pedido %s: %w", shipment.OrderID, err)
		}

		sent++
	}

	return sent, nil
}

// RunPendingPublisher calls PublishPending periodically until ctx ends.
func (handler *EventHandler) RunPendingPublisher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		sent, err := handler.PublishPending(attemptCtx)
		cancel()

		if err != nil {
			log.Printf("[ERRO] Reenvio de entregas pendentes: %v", err)
		} else if sent > 0 {
			log.Printf("[SUCESSO] %d entrega(s) pendente(s) reenviada(s)", sent)
		}
	}
}
