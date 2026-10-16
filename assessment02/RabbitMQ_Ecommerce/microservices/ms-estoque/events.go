package msestoque

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
	defaultCurrency     = "BRL"
	insufficientStock   = "Estoque insuficiente"
	eventHandlerTimeout = 10 * time.Second
	pendingMinimumAge   = 5 * time.Second
	pendingBatchSize    = 20
)

// ReservationStore is what the event handler needs from the database.
type ReservationStore interface {
	Reserve(ctx context.Context, orderID string, items []ReservationItem) (ReservationResult, error)
	Release(ctx context.Context, orderID string) (string, error)
	SaveResultEnvelope(ctx context.Context, orderID string, envelope string) (string, error)
	MarkResultPublished(ctx context.Context, orderID string) error
	ListPendingResults(ctx context.Context, minimumAge time.Duration, limit int) ([]PendingResult, error)
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
	store     ReservationStore
	signer    EventSigner
	publisher EnvelopePublisher
}

func NewEventHandler(
	store ReservationStore,
	signer EventSigner,
	publisher EnvelopePublisher,
) *EventHandler {
	return &EventHandler{
		store:     store,
		signer:    signer,
		publisher: publisher,
	}
}

// Handle processes one verified envelope. A nil return lets the consumer Ack;
// errors marked with rabbitmq.Permanent are rejected, any other error is
// retried by the reliable consumer.
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
		return rabbitmq.Permanent(fmt.Errorf(
			"tipo de evento inesperado no Estoque: %s",
			envelope.EventType,
		))
	}
}

func (handler *EventHandler) handleOrderCreated(
	ctx context.Context,
	envelope events.EventEnvelope,
) error {
	var payload events.WebOrderCreatedPayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return rabbitmq.Permanent(fmt.Errorf("erro ao desserializar pedido.criado: %w", err))
	}

	if payload.OrderID == "" {
		return rabbitmq.Permanent(fmt.Errorf("pedido.criado sem order_id"))
	}

	if payload.CustomerID == "" {
		return rabbitmq.Permanent(fmt.Errorf("pedido.criado %s sem customer_id", payload.OrderID))
	}

	if len(payload.Items) == 0 {
		return rabbitmq.Permanent(fmt.Errorf("pedido.criado %s sem itens", payload.OrderID))
	}

	items := make([]ReservationItem, 0, len(payload.Items))

	for _, item := range payload.Items {
		items = append(items, ReservationItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		})
	}

	result, err := handler.store.Reserve(ctx, payload.OrderID, items)
	if errors.Is(err, ErrInvalidOrder) {
		return rabbitmq.Permanent(fmt.Errorf("pedido %s: %w", payload.OrderID, err))
	}

	if err != nil {
		return fmt.Errorf(
			"não foi possível reservar o pedido %s: %w",
			payload.OrderID,
			err,
		)
	}

	if result.Status == ReservationReleased {
		log.Printf("[AVISO] Pedido %s já cancelado; reserva ignorada", payload.OrderID)
		return nil
	}

	if result.Published {
		log.Printf("[AVISO] Resultado do pedido %s já publicado; repetição ignorada", payload.OrderID)
		return nil
	}

	storedEnvelope := result.Envelope

	if storedEnvelope == "" {
		eventType, outgoing, ok := resultEvent(payload, result)
		if !ok {
			// Linha antiga, reentregue depois de o estoque ser reposto: sem
			// itens em falta o Gateway rejeitaria o evento.
			log.Printf(
				"[AVISO] Pedido %s registrado como indisponível sem faltas gravadas; nada a publicar",
				payload.OrderID,
			)
			return nil
		}

		signed, err := handler.signer.Sign(eventType, outgoing)
		if err != nil {
			return fmt.Errorf("erro ao assinar %s do pedido %s: %w", eventType, payload.OrderID, err)
		}

		encoded, err := json.Marshal(signed)
		if err != nil {
			return fmt.Errorf("erro ao serializar %s: %w", eventType, err)
		}

		storedEnvelope, err = handler.store.SaveResultEnvelope(ctx, payload.OrderID, string(encoded))
		if err != nil {
			return err
		}
	}

	if err := handler.publish(ctx, payload.OrderID, storedEnvelope); err != nil {
		log.Printf(
			"[AVISO] Resultado do pedido %s pendente; será reenviado: %v",
			payload.OrderID,
			err,
		)
	}

	return nil
}

// resultEvent builds the event that answers pedido.criado for a new result.
func resultEvent(
	payload events.WebOrderCreatedPayload,
	result ReservationResult,
) (string, any, bool) {
	if result.Status == ReservationReserved {
		currency := payload.Currency
		if currency == "" {
			currency = defaultCurrency
		}

		return events.OrderStockConfirmed, events.WebStockConfirmedPayload{
			OrderID:    payload.OrderID,
			CustomerID: payload.CustomerID,
			Total:      payload.Total,
			Currency:   currency,
			ReservedAt: result.ReservedAt,
		}, true
	}

	if len(result.Shortfalls) == 0 {
		return "", nil, false
	}

	unavailableItems := make([]events.WebUnavailableItem, 0, len(result.Shortfalls))

	for _, shortfall := range result.Shortfalls {
		unavailableItems = append(unavailableItems, events.WebUnavailableItem{
			ProductID:         shortfall.ProductID,
			RequestedQuantity: shortfall.RequestedQuantity,
			AvailableQuantity: shortfall.AvailableQuantity,
		})
	}

	return events.StockUnavailable, events.WebStockUnavailablePayload{
		OrderID:          payload.OrderID,
		CustomerID:       payload.CustomerID,
		UnavailableItems: unavailableItems,
		Reason:           insufficientStock,
	}, true
}

// publish sends a stored envelope (same event_id on every attempt) and marks
// the result only after the broker confirmation.
func (handler *EventHandler) publish(ctx context.Context, orderID string, stored string) error {
	var envelope events.EventEnvelope

	if err := json.Unmarshal([]byte(stored), &envelope); err != nil {
		return fmt.Errorf("envelope gravado inválido: %w", err)
	}

	if err := handler.publisher.Publish(ctx, envelope); err != nil {
		return err
	}

	if err := handler.store.MarkResultPublished(ctx, orderID); err != nil {
		// O evento saiu; no pior caso será reenviado com o mesmo event_id.
		log.Printf("[ERRO] Marcação do resultado do pedido %s: %v", orderID, err)
	}

	if envelope.EventType == events.StockUnavailable {
		printBanner("ESTOQUE INDISPONÍVEL - PEDIDO - %s", orderID)
		log.Printf("[AVISO] Estoque indisponível para o pedido %s", orderID)
	} else {
		printBanner("ESTOQUE VERIFICADO - PEDIDO - %s", orderID)
		log.Printf("[SUCESSO] Pedido %s reservado", orderID)
	}

	return nil
}

func (handler *EventHandler) handleOrderDeleted(
	ctx context.Context,
	envelope events.EventEnvelope,
) error {
	var payload events.WebOrderDeletedPayload

	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return rabbitmq.Permanent(fmt.Errorf("erro ao desserializar pedido.excluido: %w", err))
	}

	if payload.OrderID == "" {
		return rabbitmq.Permanent(fmt.Errorf("pedido.excluido sem order_id"))
	}

	status, err := handler.store.Release(ctx, payload.OrderID)
	if err != nil {
		return fmt.Errorf(
			"erro ao liberar reserva do pedido %s: %w",
			payload.OrderID,
			err,
		)
	}

	printBanner("PEDIDO EXCLUÍDO - PEDIDO - %s", payload.OrderID)
	log.Printf(
		"[SUCESSO] Exclusão do pedido %s tratada: situação %s, motivo %s",
		payload.OrderID,
		status,
		payload.Reason,
	)

	return nil
}

// PublishPending resends stored results not confirmed by the broker. It stops
// at the first broker failure and returns how many were sent.
func (handler *EventHandler) PublishPending(ctx context.Context) (int, error) {
	pending, err := handler.store.ListPendingResults(ctx, pendingMinimumAge, pendingBatchSize)
	if err != nil {
		return 0, err
	}

	sent := 0

	for _, result := range pending {
		if err := handler.publish(ctx, result.OrderID, result.Envelope); err != nil {
			return sent, fmt.Errorf("reenvio do pedido %s: %w", result.OrderID, err)
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
			log.Printf("[ERRO] Reenvio de resultados pendentes: %v", err)
		} else if sent > 0 {
			log.Printf("[SUCESSO] %d resultado(s) pendente(s) reenviado(s)", sent)
		}
	}
}
