package mspagamento

import (
	"context"
	"log"
	"time"
)

// ExpiryStore is what the expiry worker needs from the database.
type ExpiryStore interface {
	ExpireNextCharge(ctx context.Context, grace time.Duration) (Charge, bool, error)
}

// ExpireDue expires every overdue pending charge and publishes, for each one,
// pagamento.recusado with reason CHECKOUT_EXPIRED. It returns how many charges
// expired.
func ExpireDue(
	ctx context.Context,
	store ExpiryStore,
	outbox *Outbox,
	grace time.Duration,
) (int, error) {
	expired := 0

	for {
		charge, found, err := store.ExpireNextCharge(ctx, grace)
		if err != nil {
			return expired, err
		}

		if !found {
			return expired, nil
		}

		expired++

		log.Printf("[AVISO] Checkout do pedido %s expirou sem pagamento", charge.OrderID)

		// Uma falha aqui deixa o resultado pendente; a rotina de reenvio publica.
		if charge.CheckoutPublished {
			if err := outbox.PublishResult(ctx, charge); err != nil {
				log.Printf("[ERRO] Resultado da expiração do pedido %s: %v", charge.OrderID, err)
			}
		}
	}
}

// RunChargeExpiration calls ExpireDue periodically until ctx ends.
func RunChargeExpiration(
	ctx context.Context,
	store ExpiryStore,
	outbox *Outbox,
	interval time.Duration,
	grace time.Duration,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		if _, err := ExpireDue(attemptCtx, store, outbox, grace); err != nil {
			log.Printf("[ERRO] Expiração de cobranças: %v", err)
		}
		cancel()
	}
}
