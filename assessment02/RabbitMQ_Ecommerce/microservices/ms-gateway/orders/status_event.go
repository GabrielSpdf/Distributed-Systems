package orders

import (
	"encoding/json"
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"
)

// StatusEvent reúne os dados validados de um evento de atualização.
type StatusEvent struct {
	EventType  string
	OrderID    string
	CustomerID string

	StockConfirmed   *events.WebStockConfirmedPayload
	StockUnavailable *events.WebStockUnavailablePayload
	Checkout         *events.WebPaymentCheckoutPayload
	Payment          *events.WebPaymentResultPayload
	Shipped          *events.WebOrderShippedPayload
}

func ReadStatusEvent(
	eventType string,
	data json.RawMessage,
) (StatusEvent, error) {
	result := StatusEvent{EventType: eventType}

	switch eventType {
	case events.OrderStockConfirmed:
		payload, err := ReadStockConfirmedPayload(data)
		if err != nil {
			return StatusEvent{}, err
		}
		result.OrderID = payload.OrderID
		result.CustomerID = payload.CustomerID
		result.StockConfirmed = &payload

	case events.StockUnavailable:
		payload, err := ReadStockUnavailablePayload(data)
		if err != nil {
			return StatusEvent{}, err
		}
		result.OrderID = payload.OrderID
		result.CustomerID = payload.CustomerID
		result.StockUnavailable = &payload

	case events.PaymentCheckoutAvailable:
		payload, err := ReadPaymentCheckoutPayload(data)
		if err != nil {
			return StatusEvent{}, err
		}
		result.OrderID = payload.OrderID
		result.CustomerID = payload.CustomerID
		result.Checkout = &payload

	case events.PaymentApproved, events.PaymentRefused:
		payload, err := ReadPaymentResultPayload(data, eventType)
		if err != nil {
			return StatusEvent{}, err
		}
		result.OrderID = payload.OrderID
		result.CustomerID = payload.CustomerID
		result.Payment = &payload

	case events.OrderShipped:
		payload, err := ReadOrderShippedPayload(data)
		if err != nil {
			return StatusEvent{}, err
		}
		result.OrderID = payload.OrderID
		result.CustomerID = payload.CustomerID
		result.Shipped = &payload

	default:
		return StatusEvent{}, fmt.Errorf(
			"evento de atualização desconhecido: %s",
			eventType,
		)
	}

	return result, nil
}
