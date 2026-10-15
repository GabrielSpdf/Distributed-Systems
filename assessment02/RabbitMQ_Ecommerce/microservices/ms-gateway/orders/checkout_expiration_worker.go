package orders

import (
	"context"
	"log"
	"time"
)

func (repository *Repository) RunCheckoutExpiration(ctx context.Context) {
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		expired, err := repository.ExpireNextCheckout(attemptCtx)
		cancel()

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			log.Printf("[ERRO] Expiração de checkout: %v", err)
		} else if expired {
			log.Print("[INFO] Pedido expirado; compensação registrada na outbox")
		}

		if err != nil || !expired {
			if !waitStatusLoop(ctx, 5*time.Second) {
				return
			}
		}
	}
}
