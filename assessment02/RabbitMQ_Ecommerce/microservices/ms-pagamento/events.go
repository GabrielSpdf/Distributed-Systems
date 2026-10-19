package mspagamento

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
	eventHandlerTimeout = 30 * time.Second
)

// CheckoutStore is what the event handler needs from the database.
type CheckoutStore interface {
	OutboxStore
	FindChargeByOrder(ctx context.Context, orderID string) (Charge, bool, error)
	SaveCharge(ctx context.Context, charge Charge) (Charge, error)
}

// ChargeCreator creates charges in the Mock de Pagamento.
type ChargeCreator interface {
	CreateCharge(ctx context.Context, orderID string, amount float64, currency string) (MockCharge, error)
}

type EventHandler struct {
	store   CheckoutStore
	creator ChargeCreator
	outbox  *Outbox
}

func NewEventHandler(
	store CheckoutStore,
	creator ChargeCreator,
	bus EventBus,
) *EventHandler {
	return &EventHandler{
		store:   store,
		creator: creator,
		outbox:  NewOutbox(store, bus),
	}
}

// Handle processes one verified pedido.estoque_ok. A nil return lets the
// consumer Ack; errors marked with rabbitmq.Permanent are rejected and any
// other error (database, Mock) is retried by the reliable consumer.
func (handler *EventHandler) Handle(envelope events.EventEnvelope) error {
	if envelope.EventType != events.OrderStockConfirmed {
		return rabbitmq.Permanent(fmt.Errorf(
			"tipo de evento inesperado no Pagamento: %s",
			envelope.EventType,
		))
	}

	ctx, cancel := context.WithTimeout(context.Background(), eventHandlerTimeout)
	defer cancel()

	payload, err := readStockConfirmed(envelope.Payload)
	if err != nil {
		return rabbitmq.Permanent(err)
	}

	charge, exists, err := handler.store.FindChargeByOrder(ctx, payload.OrderID)
	if err != nil {
		return err
	}

	if !exists {
		mockCharge, err := handler.creator.CreateCharge(
			ctx,
			payload.OrderID,
			payload.Total,
			payload.Currency,
		)
		if err != nil {
			return fmt.Errorf("pedido %s: %w", payload.OrderID, err)
		}

		charge, err = handler.store.SaveCharge(ctx, Charge{
			ID:          mockCharge.ChargeID,
			OrderID:     payload.OrderID,
			CustomerID:  payload.CustomerID,
			Amount:      payload.Total,
			Currency:    payload.Currency,
			Status:      ChargePending,
			CheckoutURL: mockCharge.CheckoutURL,
			ExpiresAt:   mockCharge.ExpiresAt,
		})
		if err != nil {
			if errors.Is(err, ErrConflictingCharge) {
				return rabbitmq.Permanent(err)
			}
			return err
		}

		log.Printf("[SUCESSO] Cobrança %s criada para o pedido %s", charge.ID, charge.OrderID)
	}

	if !sameChargeBusinessData(charge, Charge{
		OrderID: payload.OrderID, CustomerID: payload.CustomerID,
		Amount: payload.Total, Currency: payload.Currency,
	}) {
		return rabbitmq.Permanent(fmt.Errorf("%w: pedido %s", ErrConflictingCharge, payload.OrderID))
	}

	if charge.CheckoutPublished {
		log.Printf("[AVISO] Checkout do pedido %s já publicado; repetição ignorada", charge.OrderID)
		return nil
	}

	return handler.outbox.PublishCheckout(ctx, charge)
}

func readStockConfirmed(data json.RawMessage) (events.WebStockConfirmedPayload, error) {
	var payload events.WebStockConfirmedPayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("%w: %v", ErrInvalidStockResult, err)
	}

	if err := events.ValidateWebStockConfirmed(payload); err != nil {
		return payload, fmt.Errorf("%w: %v", ErrInvalidStockResult, err)
	}

	return payload, nil
}
