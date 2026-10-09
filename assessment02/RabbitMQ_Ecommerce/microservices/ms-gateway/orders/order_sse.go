package orders

import (
	"encoding/json"
	"sync"
	"time"

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

	repository.notifyOrderStatus(
		event.CustomerID,
		event.OrderID,
		status,
		eventID,
	)

	if event.Checkout != nil {
		repository.notifyCheckoutAvailable(
			event.CustomerID,
			eventID,
			*event.Checkout,
		)
	}
}

func (repository *Repository) notifyCheckoutAvailable(
	userID string,
	eventID string,
	checkout events.WebPaymentCheckoutPayload,
) {
	payload := struct {
		OrderID     string    `json:"orderId"`
		CheckoutURL string    `json:"checkoutUrl"`
		ExpiresAt   time.Time `json:"expiresAt"`
	}{
		OrderID:     checkout.OrderID,
		CheckoutURL: checkout.CheckoutURL,
		ExpiresAt:   checkout.ExpiresAt.UTC(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	repository.sseHub.publish(userID, orderSSEEvent{
		Name: "payment.checkout.available",
		ID:   eventID,
		Data: data,
	})
}
