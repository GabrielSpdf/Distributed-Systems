package events

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Limites compatíveis com NUMERIC(12,2) e INTEGER dos bancos dos serviços.
const MaxOrderTotalCents int64 = 999999999999
const MaxOrderItemQuantity = 1<<31 - 1

// PositiveMoneyCents não arredonda valores de negócio com casas extras.
// A tolerância cobre apenas ruído de representação do float64.
func PositiveMoneyCents(value float64) (int64, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 ||
		value > float64(MaxOrderTotalCents)/100 {
		return 0, fmt.Errorf("valor deve ser positivo, finito e dentro do limite permitido")
	}
	cents := int64(math.Round(value * 100))
	if cents < 1 || cents > MaxOrderTotalCents ||
		math.Abs(value-float64(cents)/100) > 0.000000001 {
		return 0, fmt.Errorf("valor deve ter no máximo duas casas decimais")
	}
	return cents, nil
}

// OrderItemTotalCents valida os dados do item antes de qualquer reserva.
func OrderItemTotalCents(item WebOrderItem) (int64, error) {
	if strings.TrimSpace(item.ProductID) == "" || len(item.ProductID) > 64 ||
		strings.TrimSpace(item.ProductID) != item.ProductID {
		return 0, fmt.Errorf("identificador do produto inválido")
	}
	if strings.TrimSpace(item.ProductName) == "" || utf8.RuneCountInString(item.ProductName) > 160 {
		return 0, fmt.Errorf("nome do produto inválido")
	}
	if item.Quantity <= 0 || item.Quantity > MaxOrderItemQuantity {
		return 0, fmt.Errorf("quantidade deve ser positiva e dentro do limite permitido")
	}
	unitCents, err := PositiveMoneyCents(item.UnitPrice)
	if err != nil {
		return 0, fmt.Errorf("preço inválido: %w", err)
	}
	if int64(item.Quantity) > MaxOrderTotalCents/unitCents {
		return 0, fmt.Errorf("valor do item excede o limite permitido")
	}
	return unitCents * int64(item.Quantity), nil
}

func validateOrderParties(orderID, customerID string) error {
	if strings.TrimSpace(orderID) == "" || len(orderID) > 32 ||
		strings.TrimSpace(orderID) != orderID ||
		strings.ContainsAny(orderID, "\r\n\t") {
		return fmt.Errorf("identificador do pedido inválido")
	}
	if strings.TrimSpace(customerID) == "" {
		return fmt.Errorf("cliente é obrigatório")
	}
	return nil
}

func ValidateWebOrderCreated(payload WebOrderCreatedPayload) error {
	if err := validateOrderParties(payload.OrderID, payload.CustomerID); err != nil {
		return err
	}
	if payload.Currency != "BRL" {
		return fmt.Errorf("moeda deve ser BRL")
	}
	totalCents, err := PositiveMoneyCents(payload.Total)
	if err != nil {
		return fmt.Errorf("total inválido: %w", err)
	}
	if len(payload.Items) == 0 {
		return fmt.Errorf("pedido deve ter itens")
	}
	seen := make(map[string]bool, len(payload.Items))
	var sum int64
	for _, item := range payload.Items {
		if seen[item.ProductID] {
			return fmt.Errorf("produto repetido no pedido")
		}
		seen[item.ProductID] = true
		lineCents, err := OrderItemTotalCents(item)
		if err != nil {
			return fmt.Errorf("item %s inválido: %w", item.ProductID, err)
		}
		if lineCents > MaxOrderTotalCents-sum {
			return fmt.Errorf("valor do pedido excede o limite permitido")
		}
		sum += lineCents
	}
	if totalCents != sum {
		return fmt.Errorf("total não corresponde à soma dos itens")
	}
	return nil
}

func ValidateWebStockConfirmed(payload WebStockConfirmedPayload) error {
	if err := validateOrderParties(payload.OrderID, payload.CustomerID); err != nil {
		return err
	}
	if _, err := PositiveMoneyCents(payload.Total); err != nil {
		return fmt.Errorf("total inválido: %w", err)
	}
	if payload.Currency != "BRL" {
		return fmt.Errorf("moeda deve ser BRL")
	}
	if payload.ReservedAt.IsZero() {
		return fmt.Errorf("data da reserva é obrigatória")
	}
	return nil
}
