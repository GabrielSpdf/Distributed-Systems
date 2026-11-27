package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5"
)

// ExpireNextCheckout é a operação histórica, não executada pelo Gateway.
// Somente o MS Pagamento decide a expiração. Mantida como referência e teste.
// Pode cancelar uma aprovação persistida que ainda não tenha sido publicada.
// Retorna false quando não há pedido elegível.
//
// Deprecated: não chamar sem arbitragem coordenada com o MS Pagamento.
func (repository *Repository) ExpireNextCheckout(
	ctx context.Context,
) (bool, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar expiração: %w", err)
	}
	defer tx.Rollback(ctx)

	var databaseID, displayID, userID string

	err = tx.QueryRow(ctx, `
		SELECT o.id::text, o.display_id, o.user_id::text
		FROM gateway.orders AS o
		WHERE o.status = $1
		  AND o.is_deleted = FALSE
		  AND o.checkout_expires_at <= clock_timestamp() - INTERVAL '10 minutes'
		  AND NOT EXISTS (
		      SELECT 1
		      FROM gateway.event_inbox AS inbox
		      WHERE inbox.order_id = o.display_id
		        AND inbox.customer_id = o.user_id
		        AND inbox.event_type IN ($2, $3)
		        AND inbox.status IN ('RECEBIDO', 'PENDENTE_REVISAO')
		  )
		ORDER BY o.checkout_expires_at, o.id
		LIMIT 1
		FOR UPDATE OF o SKIP LOCKED
	`,
		StatusAwaitingPayment,
		events.PaymentApproved,
		events.PaymentRefused,
	).Scan(&databaseID, &displayID, &userID)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao selecionar checkout vencido: %w", err)
	}

	var cancelledAt time.Time
	err = tx.QueryRow(ctx, `
		UPDATE gateway.orders
		SET status = $2,
		    updated_at = clock_timestamp()
		WHERE id = $1
		RETURNING updated_at
	`, databaseID, StatusCancelled).Scan(&cancelledAt)
	if err != nil {
		return false, fmt.Errorf("erro ao expirar pedido: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.order_status_history (
			order_id, status, created_at
		)
		VALUES ($1, $2, $3)
	`, databaseID, StatusCancelled, cancelledAt)
	if err != nil {
		return false, fmt.Errorf("erro ao registrar expiração: %w", err)
	}

	err = repository.enqueueOrderDeletedTx(
		ctx,
		tx,
		databaseID,
		events.WebOrderDeletedPayload{
			OrderID:     displayID,
			CustomerID:  userID,
			Reason:      events.ReasonCheckoutExpired,
			CancelledAt: cancelledAt.UTC(),
		},
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao confirmar expiração: %w", err)
	}

	// Só notifica a interface depois da confirmação no banco.
	repository.notifyOrderStatus(
		userID,
		displayID,
		StatusCancelled,
		"",
	)

	return true, nil
}
