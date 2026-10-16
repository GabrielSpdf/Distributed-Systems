package msentrega

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

// newInvoiceID simula a nota fiscal: NF-AAAAMMDD-NNNNNN.
func newInvoiceID(now time.Time) (string, error) {
	digits, err := randomDigits(6)
	if err != nil {
		return "", err
	}

	return "NF-" + now.UTC().Format("20060102") + "-" + digits, nil
}

// newTrackingCode simula o rastreio no formato dos Correios: BR + 9 dígitos + BR.
func newTrackingCode() (string, error) {
	digits, err := randomDigits(9)
	if err != nil {
		return "", err
	}

	return "BR" + digits + "BR", nil
}

func randomDigits(length int) (string, error) {
	var builder strings.Builder

	for range length {
		digit, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("erro ao gerar identificador: %w", err)
		}

		builder.WriteString(digit.String())
	}

	return builder.String(), nil
}

// readPaymentApproved aplica as mesmas regras que o Gateway usa para o
// resultado do pagamento, antes de gerar qualquer entrega.
func readPaymentApproved(data json.RawMessage) (events.WebPaymentResultPayload, error) {
	var payload events.WebPaymentResultPayload

	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("%w: %v", ErrInvalidPayment, err)
	}

	if strings.TrimSpace(payload.OrderID) == "" ||
		strings.TrimSpace(payload.CustomerID) == "" ||
		strings.TrimSpace(payload.ChargeID) == "" {
		return payload, fmt.Errorf("%w: pedido, cliente e cobrança são obrigatórios", ErrInvalidPayment)
	}

	if payload.Amount <= 0 || math.IsNaN(payload.Amount) || math.IsInf(payload.Amount, 0) {
		return payload, fmt.Errorf("%w: valor deve ser positivo", ErrInvalidPayment)
	}

	if payload.Currency != "BRL" {
		return payload, fmt.Errorf("%w: moeda deve ser BRL", ErrInvalidPayment)
	}

	if payload.ProcessedAt.IsZero() {
		return payload, fmt.Errorf("%w: data do processamento é obrigatória", ErrInvalidPayment)
	}

	return payload, nil
}
