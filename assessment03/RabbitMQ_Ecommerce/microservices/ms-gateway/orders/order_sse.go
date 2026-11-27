package orders

import (
	"context"
	"encoding/json"
	"sync"

	"RabbitMQ_Ecommerce/utils/events"
)

type orderSSEEvent struct {
	Name string
	ID   string
	Data json.RawMessage
}

type orderSSEHub struct {
	mutex       sync.Mutex
	subscribers map[string]map[chan orderSSEEvent]struct{}
}

func newOrderSSEHub() *orderSSEHub {
	return &orderSSEHub{
		subscribers: make(map[string]map[chan orderSSEEvent]struct{}),
	}
}

func (hub *orderSSEHub) subscribe(
	userID string,
) (<-chan orderSSEEvent, func()) {
	channel := make(chan orderSSEEvent, 16)

	hub.mutex.Lock()
	if hub.subscribers[userID] == nil {
		hub.subscribers[userID] = make(map[chan orderSSEEvent]struct{})
	}
	hub.subscribers[userID][channel] = struct{}{}
	hub.mutex.Unlock()

	unsubscribe := func() {
		hub.mutex.Lock()
		defer hub.mutex.Unlock()

		subscribers := hub.subscribers[userID]
		if _, exists := subscribers[channel]; !exists {
			return
		}

		delete(subscribers, channel)
		close(channel)

		if len(subscribers) == 0 {
			delete(hub.subscribers, userID)
		}
	}

	return channel, unsubscribe
}

func (hub *orderSSEHub) publish(
	userID string,
	event orderSSEEvent,
) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()

	subscribers := hub.subscribers[userID]

	for channel := range subscribers {
		select {
		case channel <- event:
		default:
			delete(subscribers, channel)
			close(channel)
		}
	}

	if len(subscribers) == 0 {
		delete(hub.subscribers, userID)
	}
}

func (repository *Repository) notifyOrderStatus(
	userID string,
	orderID string,
	status string,
	eventID string,
) {
	payload := struct {
		OrderID string `json:"orderId"`
		Status  string `json:"status"`
	}{
		OrderID: orderID,
		Status:  status,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	repository.sseHub.publish(userID, orderSSEEvent{
		Name: "order.status.changed",
		ID:   eventID,
		Data: data,
	})
}

func (repository *Repository) notifyAppliedStatusEvent(
	eventID string,
	eventType string,
	data json.RawMessage,
) {
	event, err := ReadStatusEvent(eventType, data)
	if err != nil {
		return
	}

	statuses := map[string]string{
		events.OrderStockConfirmed:      StatusStockConfirmed,
		events.StockUnavailable:         StatusStockUnavailable,
		events.PaymentCheckoutAvailable: StatusAwaitingPayment,
		events.PaymentApproved:          StatusPaymentApproved,
		events.PaymentRefused:           StatusPaymentRefused,
		events.OrderShipped:             StatusShipped,
	}

	status, exists := statuses[eventType]
	if !exists {
		return
	}

	if isCheckoutExpiredRefusal(eventType, event) {
		status = StatusCancelled
	}

	repository.notifyOrderStatus(
		event.CustomerID,
		event.OrderID,
		status,
		eventID,
	)
}

type OrderStatusUpdate struct {
	EventID string
	OrderID string `json:"orderId"`
	Status  string `json:"status"`
}

func (repository *Repository) SubscribeOrderStatus(
	ctx context.Context,
	userID string,
) <-chan OrderStatusUpdate {
	source, unsubscribe := repository.sseHub.subscribe(userID)
	updates := make(chan OrderStatusUpdate)

	go func() {
		defer close(updates)
		defer unsubscribe()

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-source:
				if !ok {
					return
				}
				if event.Name != "order.status.changed" {
					continue
				}

				var update OrderStatusUpdate
				if err := json.Unmarshal(event.Data, &update); err != nil {
					continue
				}
				update.EventID = event.ID

				select {
				case updates <- update:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return updates
}
