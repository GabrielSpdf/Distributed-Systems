package orders

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"

	"RabbitMQ_Ecommerce/utils/events"
)

func ReadStockConfirmedPayload(
	data json.RawMessage,
) (events.WebStockConfirmedPayload, error) {
	var payload events.WebStockConfirmedPayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf(
			"JSON inválido em pedido.estoque_ok: %w",
			err,
		)
	}

	if strings.TrimSpace(payload.OrderID) == "" ||
		strings.TrimSpace(payload.CustomerID) == "" {
		return payload, fmt.Errorf("pedido e cliente são obrigatórios")
	}

	if payload.Total <= 0 ||
		math.IsNaN(payload.Total) ||
		math.IsInf(payload.Total, 0) {
		return payload, fmt.Errorf("total deve ser positivo e finito")
	}

	if payload.Currency != "BRL" {
		return payload, fmt.Errorf("moeda deve ser BRL")
	}

	if payload.ReservedAt.IsZero() {
		return payload, fmt.Errorf("data da reserva é obrigatória")
	}

	return payload, nil
}

func ReadStockUnavailablePayload(
	data json.RawMessage,
) (events.WebStockUnavailablePayload, error) {
	var payload events.WebStockUnavailablePayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf(
			"JSON inválido em estoque.indisponivel: %w",
			err,
		)
	}

	if strings.TrimSpace(payload.OrderID) == "" ||
		strings.TrimSpace(payload.CustomerID) == "" {
		return payload, fmt.Errorf("pedido e cliente são obrigatórios")
	}

	if strings.TrimSpace(payload.Reason) == "" {
		return payload, fmt.Errorf("motivo é obrigatório")
	}

	if len(payload.UnavailableItems) == 0 {
		return payload, fmt.Errorf("informe os itens indisponíveis")
	}

	seenProducts := make(map[string]bool)

	for _, item := range payload.UnavailableItems {
		if strings.TrimSpace(item.ProductID) == "" {
			return payload, fmt.Errorf("produto é obrigatório")
		}

		if seenProducts[item.ProductID] {
			return payload, fmt.Errorf("produto indisponível repetido")
		}
		seenProducts[item.ProductID] = true

		if item.RequestedQuantity <= 0 {
			return payload, fmt.Errorf("quantidade solicitada deve ser positiva")
		}

		if item.AvailableQuantity < 0 ||
			item.AvailableQuantity >= item.RequestedQuantity {
			return payload, fmt.Errorf(
				"quantidade disponível deve ser não negativa e menor que a solicitada",
			)
		}
	}

	return payload, nil
}

func ReadPaymentCheckoutPayload(
	data json.RawMessage,
) (events.WebPaymentCheckoutPayload, error) {
	var payload events.WebPaymentCheckoutPayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf(
			"JSON inválido em pagamento.checkout_disponivel: %w",
			err,
		)
	}

	if strings.TrimSpace(payload.OrderID) == "" ||
		strings.TrimSpace(payload.CustomerID) == "" {
		return payload, fmt.Errorf("pedido e cliente são obrigatórios")
	}

	if strings.TrimSpace(payload.ChargeID) == "" {
		return payload, fmt.Errorf("identificador da cobrança é obrigatório")
	}

	checkoutURL, err := url.Parse(payload.CheckoutURL)
	if err != nil {
		return payload, fmt.Errorf("URL do checkout inválida: %w", err)
	}

	if (checkoutURL.Scheme != "http" && checkoutURL.Scheme != "https") ||
		checkoutURL.Hostname() == "" ||
		checkoutURL.User != nil {
		return payload, fmt.Errorf(
			"checkout deve ter URL absoluta HTTP/HTTPS sem credenciais",
		)
	}

	if payload.ExpiresAt.IsZero() {
		return payload, fmt.Errorf("data de expiração é obrigatória")
	}

	return payload, nil
}

func ReadPaymentResultPayload(
	data json.RawMessage,
	eventType string,
) (events.WebPaymentResultPayload, error) {
	var payload events.WebPaymentResultPayload

	if eventType != events.PaymentApproved &&
		eventType != events.PaymentRefused {
		return payload, fmt.Errorf(
			"tipo de resultado de pagamento inválido: %s",
			eventType,
		)
	}

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf(
			"JSON inválido em %s: %w",
			eventType,
			err,
		)
	}

	if strings.TrimSpace(payload.OrderID) == "" ||
		strings.TrimSpace(payload.CustomerID) == "" ||
		strings.TrimSpace(payload.ChargeID) == "" {
		return payload, fmt.Errorf(
			"pedido, cliente e cobrança são obrigatórios",
		)
	}

	if payload.Amount <= 0 ||
		math.IsNaN(payload.Amount) ||
		math.IsInf(payload.Amount, 0) {
		return payload, fmt.Errorf(
			"valor do pagamento deve ser positivo e finito",
		)
	}

	if payload.Currency != "BRL" {
		return payload, fmt.Errorf("moeda deve ser BRL")
	}

	if payload.ProcessedAt.IsZero() {
		return payload, fmt.Errorf("data do processamento é obrigatória")
	}

	if eventType == events.PaymentRefused &&
		strings.TrimSpace(payload.Reason) == "" {
		return payload, fmt.Errorf("motivo da recusa é obrigatório")
	}

	return payload, nil
}

func ReadOrderShippedPayload(
	data json.RawMessage,
) (events.WebOrderShippedPayload, error) {
	var payload events.WebOrderShippedPayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf(
			"JSON inválido em pedido.enviado: %w",
			err,
		)
	}

	if strings.TrimSpace(payload.OrderID) == "" ||
		strings.TrimSpace(payload.CustomerID) == "" {
		return payload, fmt.Errorf("pedido e cliente são obrigatórios")
	}

	if strings.TrimSpace(payload.InvoiceID) == "" {
		return payload, fmt.Errorf("identificador da nota fiscal é obrigatório")
	}

	if strings.TrimSpace(payload.TrackingCode) == "" {
		return payload, fmt.Errorf("código de rastreamento é obrigatório")
	}

	if payload.ShippedAt.IsZero() {
		return payload, fmt.Errorf("data de envio é obrigatória")
	}

	return payload, nil
}
