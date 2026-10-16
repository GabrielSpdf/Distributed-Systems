package mspagamento

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

const (
	pendingMinimumAge = 5 * time.Second
	pendingBatchSize  = 20
)

// EventBus signs outgoing events and publishes signed envelopes, returning
// only after the broker confirms them.
type EventBus interface {
	Sign(eventType string, payload any) (events.EventEnvelope, error)
	Publish(ctx context.Context, envelope events.EventEnvelope) error
}

// OutboxStore is what the Outbox needs from the database.
type OutboxStore interface {
	SaveCheckoutEnvelope(ctx context.Context, chargeID string, envelope string) (string, error)
	SaveResultEnvelope(ctx context.Context, chargeID string, envelope string) (string, error)
	MarkCheckoutPublished(ctx context.Context, chargeID string) error
	MarkResultPublished(ctx context.Context, chargeID string) error
	ListPendingCheckouts(ctx context.Context, minimumAge time.Duration, limit int) ([]Charge, error)
	ListPendingResults(ctx context.Context, minimumAge time.Duration, limit int) ([]Charge, error)
}

// Outbox publishes the two events of a charge: the checkout and the result.
// Each event is signed once and stored before publishing, so every resend uses
// the same event_id; a charge is marked only after the broker confirmation.
type Outbox struct {
	store OutboxStore
	bus   EventBus
}

func NewOutbox(store OutboxStore, bus EventBus) *Outbox {
	return &Outbox{
		store: store,
		bus:   bus,
	}
}

// PublishCheckout stores and sends pagamento.checkout_disponivel. It returns an
// error only when the event could not be stored; a broker failure leaves it
// pending for PublishPending.
func (outbox *Outbox) PublishCheckout(ctx context.Context, charge Charge) error {
	if charge.CheckoutPublished {
		return nil
	}

	envelope, err := outbox.checkoutEnvelope(ctx, charge)
	if err != nil {
		return err
	}

	if err := outbox.sendCheckout(ctx, charge, envelope); err != nil {
		log.Printf("[AVISO] Checkout do pedido %s pendente; será reenviado: %v", charge.OrderID, err)
	}

	return nil
}

// PublishResult stores and sends pagamento.aprovado or pagamento.recusado, with
// the same error rule as PublishCheckout.
func (outbox *Outbox) PublishResult(ctx context.Context, charge Charge) error {
	if charge.ResultPublished {
		return nil
	}

	envelope, err := outbox.resultEnvelope(ctx, charge)
	if err != nil {
		return err
	}

	if err := outbox.sendResult(ctx, charge, envelope); err != nil {
		log.Printf("[AVISO] Resultado do pedido %s pendente; será reenviado: %v", charge.OrderID, err)
	}

	return nil
}

func (outbox *Outbox) checkoutEnvelope(ctx context.Context, charge Charge) (string, error) {
	if charge.CheckoutEnvelope != "" {
		return charge.CheckoutEnvelope, nil
	}

	encoded, err := outbox.sign(events.PaymentCheckoutAvailable, events.WebPaymentCheckoutPayload{
		OrderID:     charge.OrderID,
		CustomerID:  charge.CustomerID,
		ChargeID:    charge.ID,
		CheckoutURL: charge.CheckoutURL,
		ExpiresAt:   charge.ExpiresAt,
	})
	if err != nil {
		return "", err
	}

	return outbox.store.SaveCheckoutEnvelope(ctx, charge.ID, encoded)
}

func (outbox *Outbox) resultEnvelope(ctx context.Context, charge Charge) (string, error) {
	if charge.ResultEnvelope != "" {
		return charge.ResultEnvelope, nil
	}

	eventType, payload, err := resultPayload(charge)
	if err != nil {
		return "", err
	}

	encoded, err := outbox.sign(eventType, payload)
	if err != nil {
		return "", err
	}

	return outbox.store.SaveResultEnvelope(ctx, charge.ID, encoded)
}

// resultPayload builds the result event of a decided charge.
func resultPayload(charge Charge) (string, events.WebPaymentResultPayload, error) {
	payload := events.WebPaymentResultPayload{
		OrderID:     charge.OrderID,
		CustomerID:  charge.CustomerID,
		ChargeID:    charge.ID,
		Amount:      charge.Amount,
		Currency:    charge.Currency,
		ProcessedAt: charge.ProcessedAt,
	}

	switch charge.Status {
	case ChargeApproved:
		return events.PaymentApproved, payload, nil
	case ChargeRefused, ChargeExpired:
		payload.Reason = charge.Reason
		return events.PaymentRefused, payload, nil
	default:
		return "", payload, fmt.Errorf("cobrança %s sem resultado (%s)", charge.ID, charge.Status)
	}
}

func (outbox *Outbox) sign(eventType string, payload any) (string, error) {
	envelope, err := outbox.bus.Sign(eventType, payload)
	if err != nil {
		return "", fmt.Errorf("erro ao assinar %s: %w", eventType, err)
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("erro ao serializar %s: %w", eventType, err)
	}

	return string(encoded), nil
}

func (outbox *Outbox) sendCheckout(ctx context.Context, charge Charge, stored string) error {
	if err := outbox.send(ctx, stored); err != nil {
		return err
	}

	if err := outbox.store.MarkCheckoutPublished(ctx, charge.ID); err != nil {
		// O evento saiu; no pior caso será reenviado com o mesmo event_id.
		log.Printf("[ERRO] Marcação do checkout da cobrança %s: %v", charge.ID, err)
	}

	printBanner("CHECKOUT DISPONÍVEL - PEDIDO - %s", charge.OrderID)
	log.Printf("[SUCESSO] Checkout do pedido %s publicado", charge.OrderID)

	return nil
}

func (outbox *Outbox) sendResult(ctx context.Context, charge Charge, stored string) error {
	if err := outbox.send(ctx, stored); err != nil {
		return err
	}

	if err := outbox.store.MarkResultPublished(ctx, charge.ID); err != nil {
		log.Printf("[ERRO] Marcação do resultado da cobrança %s: %v", charge.ID, err)
	}

	switch charge.Status {
	case ChargeApproved:
		printBanner("PAGAMENTO APROVADO - PEDIDO - %s", charge.OrderID)
	case ChargeExpired:
		printBanner("CHECKOUT EXPIRADO - PEDIDO - %s", charge.OrderID)
	default:
		printBanner("PAGAMENTO RECUSADO - PEDIDO - %s", charge.OrderID)
	}
	log.Printf("[SUCESSO] Pagamento do pedido %s %s", charge.OrderID, charge.Status)

	return nil
}

func (outbox *Outbox) send(ctx context.Context, stored string) error {
	var envelope events.EventEnvelope

	if err := json.Unmarshal([]byte(stored), &envelope); err != nil {
		return fmt.Errorf("envelope gravado inválido: %w", err)
	}

	return outbox.bus.Publish(ctx, envelope)
}

// PublishPending resends every stored event not confirmed by the broker:
// checkouts first, then results. It stops at the first failure.
func (outbox *Outbox) PublishPending(ctx context.Context) (int, error) {
	sent := 0

	checkouts, err := outbox.store.ListPendingCheckouts(ctx, pendingMinimumAge, pendingBatchSize)
	if err != nil {
		return sent, err
	}

	for _, charge := range checkouts {
		envelope, err := outbox.checkoutEnvelope(ctx, charge)
		if err != nil {
			return sent, err
		}

		if err := outbox.sendCheckout(ctx, charge, envelope); err != nil {
			return sent, fmt.Errorf("reenvio do checkout do pedido %s: %w", charge.OrderID, err)
		}

		sent++
	}

	results, err := outbox.store.ListPendingResults(ctx, pendingMinimumAge, pendingBatchSize)
	if err != nil {
		return sent, err
	}

	for _, charge := range results {
		envelope, err := outbox.resultEnvelope(ctx, charge)
		if err != nil {
			return sent, err
		}

		if err := outbox.sendResult(ctx, charge, envelope); err != nil {
			return sent, fmt.Errorf("reenvio do resultado do pedido %s: %w", charge.OrderID, err)
		}

		sent++
	}

	return sent, nil
}

// RunPendingPublisher calls PublishPending periodically until ctx ends.
func (outbox *Outbox) RunPendingPublisher(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		sent, err := outbox.PublishPending(attemptCtx)
		cancel()

		if err != nil {
			log.Printf("[ERRO] Reenvio de eventos de pagamento pendentes: %v", err)
		} else if sent > 0 {
			log.Printf("[SUCESSO] %d evento(s) de pagamento pendente(s) reenviado(s)", sent)
		}
	}
}
