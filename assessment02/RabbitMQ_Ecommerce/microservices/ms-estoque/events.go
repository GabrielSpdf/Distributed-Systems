package msestoque

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

const (
	defaultCurrency     = "BRL"
	insufficientStock   = "Estoque insuficiente"
	eventHandlerTimeout = 10 * time.Second
)

// ReservationStore is what the event handler needs from the database.
type ReservationStore interface {
	Reserve(ctx context.Context, orderID string, items []ReservationItem) (ReservationResult, error)
	Release(ctx context.Context, orderID string) (string, error)
}

// EventPublisher sends one event to RabbitMQ.
type EventPublisher interface {
	Publish(eventType string, payload any) error
}

type EventHandler struct {
	store     ReservationStore
	publisher EventPublisher
}

func NewEventHandler(
	store ReservationStore,
	publisher EventPublisher,
) *EventHandler {
	return &EventHandler{
		store:     store,
		publisher: publisher,
	}
}

// Handle processes one verified envelope. A nil return lets the consumer Ack.
func (handler *EventHandler) Handle(envelope events.EventEnvelope) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		eventHandlerTimeout,
	)
	defer cancel()

	switch envelope.EventType {
	case events.OrderCreated:
		return handler.handleOrderCreated(ctx, envelope)
	case events.OrderDeleted:
		return handler.handleOrderDeleted(ctx, envelope)
	default:
		return fmt.Errorf(
			"tipo de evento inesperado no Estoque: %s",
			envelope.EventType,
		)
	}
}

func (handler *EventHandler) handleOrderCreated(
	ctx context.Context,
	envelope events.EventEnvelope,
) error {
	var payload events.WebOrderCreatedPayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao desserializar pedido.criado: %w", err)
	}

	if payload.OrderID == "" {
		return fmt.Errorf("pedido.criado sem order_id")
	}

	if payload.CustomerID == "" {
		return fmt.Errorf("pedido.criado %s sem customer_id", payload.OrderID)
	}

	if len(payload.Items) == 0 {
		return fmt.Errorf("pedido.criado %s sem itens", payload.OrderID)
	}

	items := make([]ReservationItem, 0, len(payload.Items))

	for _, item := range payload.Items {
		items = append(items, ReservationItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	result, err := handler.store.Reserve(ctx, payload.OrderID, items)
	if err != nil {
		return fmt.Errorf(
			"não foi possível reservar o pedido %s: %w",
			payload.OrderID,
			err,
		)
	}

	switch result.Status {
	case ReservationReserved:
		currency := payload.Currency
		if currency == "" {
			currency = defaultCurrency
		}

		if err := handler.publisher.Publish(
			events.OrderStockConfirmed,
			events.WebStockConfirmedPayload{
				OrderID:    payload.OrderID,
				CustomerID: payload.CustomerID,
				Total:      payload.Total,
				Currency:   currency,
				ReservedAt: result.ReservedAt,
			},
		); err != nil {
			return fmt.Errorf(
				"erro ao publicar pedido.estoque_ok do pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

		log.Printf("[SUCESSO] Pedido %s reservado", payload.OrderID)

	case ReservationUnavailable:
		// Em uma reentrega o estoque pode ter sido reposto; sem itens em falta
		// o evento seria rejeitado pelo Gateway, e a primeira entrega já avisou.
		if len(result.Shortfalls) == 0 {
			log.Printf(
				"[AVISO] Pedido %s já registrado como indisponível; nada a publicar",
				payload.OrderID,
			)
			return nil
		}

		unavailableItems := make([]events.WebUnavailableItem, 0, len(result.Shortfalls))

		for _, shortfall := range result.Shortfalls {
			unavailableItems = append(unavailableItems, events.WebUnavailableItem{
				ProductID:         shortfall.ProductID,
				RequestedQuantity: shortfall.RequestedQuantity,
				AvailableQuantity: shortfall.AvailableQuantity,
			})
		}

		if err := handler.publisher.Publish(
			events.StockUnavailable,
			events.WebStockUnavailablePayload{
				OrderID:          payload.OrderID,
				CustomerID:       payload.CustomerID,
				UnavailableItems: unavailableItems,
				Reason:           insufficientStock,
			},
		); err != nil {
			return fmt.Errorf(
				"erro ao publicar estoque.indisponivel do pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

		log.Printf("[AVISO] Estoque indisponível para o pedido %s", payload.OrderID)

	default:
		log.Printf(
			"[AVISO] Pedido %s já cancelado; reserva ignorada",
			payload.OrderID,
		)
	}

	return nil
}

func (handler *EventHandler) handleOrderDeleted(
	ctx context.Context,
	envelope events.EventEnvelope,
) error {
	var payload events.WebOrderDeletedPayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("erro ao desserializar pedido.excluido: %w", err)
	}

	if payload.OrderID == "" {
		return fmt.Errorf("pedido.excluido sem order_id")
	}

	status, err := handler.store.Release(ctx, payload.OrderID)
	if err != nil {
		return fmt.Errorf(
			"erro ao liberar reserva do pedido %s: %w",
			payload.OrderID,
			err,
		)
	}

	log.Printf(
		"[SUCESSO] Exclusão do pedido %s tratada: situação %s, motivo %s",
		payload.OrderID,
		status,
		payload.Reason,
	)

	return nil
}
