package orders

import (
	"encoding/json"
	"testing"

	"RabbitMQ_Ecommerce/utils/events"
)

func TestReadStatusEvent(t *testing.T) {
	tests := []struct {
		eventType string
		payload   string
	}{
		{
			events.OrderStockConfirmed,
			`{"order_id":"PED-001","customer_id":"cliente-teste",
			  "total":4.9,"currency":"BRL",
			  "reserved_at":"2026-10-07T21:00:00Z"}`,
		},
		{
			events.StockUnavailable,
			`{"order_id":"PED-001","customer_id":"cliente-teste",
			  "reason":"Estoque insuficiente",
			  "unavailable_items":[{"product_id":"PROD-002",
			  "requested_quantity":2,"available_quantity":1}]}`,
		},
		{
			events.PaymentCheckoutAvailable,
			`{"order_id":"PED-001","customer_id":"cliente-teste",
			  "charge_id":"COB-001",
			  "checkout_url":"https://checkout.example.com/COB-001",
			  "expires_at":"2026-10-07T22:00:00Z"}`,
		},
		{
			events.PaymentApproved,
			`{"order_id":"PED-001","customer_id":"cliente-teste",
			  "charge_id":"COB-001","amount":4.9,"currency":"BRL",
			  "processed_at":"2026-10-07T21:00:00Z"}`,
		},
		{
			events.PaymentRefused,
			`{"order_id":"PED-001","customer_id":"cliente-teste",
			  "charge_id":"COB-001","amount":4.9,"currency":"BRL",
			  "reason":"Pagamento recusado",
			  "processed_at":"2026-10-07T21:00:00Z"}`,
		},
		{
			events.OrderShipped,
			`{"order_id":"PED-001","customer_id":"cliente-teste",
			  "invoice_id":"NF-001","tracking_code":"RASTREIO-001",
			  "shipped_at":"2026-10-07T21:00:00Z"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.eventType, func(t *testing.T) {
			event, err := ReadStatusEvent(
				tt.eventType,
				json.RawMessage(tt.payload),
			)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}

			if event.EventType != tt.eventType ||
				event.OrderID != "PED-001" ||
				event.CustomerID != "cliente-teste" {
				t.Fatalf("evento inesperado: %+v", event)
			}

			fields := []bool{
				event.StockConfirmed != nil,
				event.StockUnavailable != nil,
				event.Checkout != nil,
				event.Payment != nil,
				event.Shipped != nil,
			}

			count := 0
			for _, present := range fields {
				if present {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("esperava apenas um payload preenchido; recebeu %d", count)
			}

			switch tt.eventType {
			case events.OrderStockConfirmed:
				if event.StockConfirmed == nil {
					t.Fatal("payload de estoque confirmado ausente")
				}
			case events.StockUnavailable:
				if event.StockUnavailable == nil {
					t.Fatal("payload de estoque indisponível ausente")
				}
			case events.PaymentCheckoutAvailable:
				if event.Checkout == nil {
					t.Fatal("payload de checkout ausente")
				}
			case events.PaymentApproved, events.PaymentRefused:
				if event.Payment == nil {
					t.Fatal("payload de pagamento ausente")
				}
			case events.OrderShipped:
				if event.Shipped == nil {
					t.Fatal("payload de envio ausente")
				}
			}

			_, err = ReadStatusEvent(tt.eventType, json.RawMessage(`{}`))
			if err == nil {
				t.Fatal("esperava rejeição dos campos ausentes")
			}
		})
	}
}

func TestReadStatusEventRejectsUnknownType(t *testing.T) {
	_, err := ReadStatusEvent(
		"evento.desconhecido",
		json.RawMessage(`{}`),
	)
	if err == nil {
		t.Fatal("esperava rejeição do evento desconhecido")
	}
}
