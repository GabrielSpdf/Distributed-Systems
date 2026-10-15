package mspagamento

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns every query against the pagamento schema.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (repository *Repository) Ping(ctx context.Context) error {
	return repository.pool.Ping(ctx)
}

const chargeColumns = `
	id::text,
	order_id,
	COALESCE(customer_id, ''),
	amount::double precision,
	currency,
	status,
	COALESCE(checkout_url, ''),
	expires_at,
	processed_at,
	COALESCE(reason, ''),
	checkout_published_at IS NOT NULL,
	result_published_at IS NOT NULL
`

func scanCharge(row pgx.Row) (Charge, error) {
	var (
		charge      Charge
		expiresAt   *time.Time
		processedAt *time.Time
	)

	err := row.Scan(
		&charge.ID,
		&charge.OrderID,
		&charge.CustomerID,
		&charge.Amount,
		&charge.Currency,
		&charge.Status,
		&charge.CheckoutURL,
		&expiresAt,
		&processedAt,
		&charge.Reason,
		&charge.CheckoutPublished,
		&charge.ResultPublished,
	)
	if err != nil {
		return Charge{}, err
	}

	if expiresAt != nil {
		charge.ExpiresAt = expiresAt.UTC()
	}

	if processedAt != nil {
		charge.ProcessedAt = processedAt.UTC()
	}

	return charge, nil
}

// FindChargeByOrder returns the charge of an order, if there is one.
func (repository *Repository) FindChargeByOrder(
	ctx context.Context,
	orderID string,
) (Charge, bool, error) {
	charge, err := scanCharge(repository.pool.QueryRow(
		ctx,
		`SELECT `+chargeColumns+`
		   FROM pagamento.payment_charges
		  WHERE order_id = $1`,
		orderID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Charge{}, false, nil
	}

	if err != nil {
		return Charge{}, false, fmt.Errorf("erro ao consultar cobrança: %w", err)
	}

	return charge, true, nil
}

// SaveCharge stores a new charge. If the order already has one, the stored
// charge is returned unchanged.
func (repository *Repository) SaveCharge(
	ctx context.Context,
	charge Charge,
) (Charge, error) {
	if _, err := repository.pool.Exec(
		ctx,
		`INSERT INTO pagamento.payment_charges
		     (id, order_id, customer_id, amount, currency, status, checkout_url, expires_at)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (order_id) DO NOTHING`,
		charge.ID,
		charge.OrderID,
		charge.CustomerID,
		charge.Amount,
		charge.Currency,
		charge.Status,
		charge.CheckoutURL,
		charge.ExpiresAt,
	); err != nil {
		return Charge{}, fmt.Errorf("erro ao gravar cobrança: %w", err)
	}

	stored, exists, err := repository.FindChargeByOrder(ctx, charge.OrderID)
	if err != nil {
		return Charge{}, err
	}

	if !exists {
		return Charge{}, fmt.Errorf("cobrança do pedido %s não encontrada após gravação", charge.OrderID)
	}

	return stored, nil
}

func (repository *Repository) MarkCheckoutPublished(ctx context.Context, chargeID string) error {
	return repository.markPublished(ctx, chargeID, "checkout_published_at")
}

func (repository *Repository) MarkResultPublished(ctx context.Context, chargeID string) error {
	return repository.markPublished(ctx, chargeID, "result_published_at")
}

// markPublished only receives the two column names above, never user input.
func (repository *Repository) markPublished(
	ctx context.Context,
	chargeID string,
	column string,
) error {
	if _, err := repository.pool.Exec(
		ctx,
		`UPDATE pagamento.payment_charges
		    SET `+column+` = COALESCE(`+column+`, NOW()),
		        updated_at = NOW()
		  WHERE id = $1::uuid`,
		chargeID,
	); err != nil {
		return fmt.Errorf("erro ao marcar publicação da cobrança %s: %w", chargeID, err)
	}

	return nil
}

// ApplyWebhook records the webhook event and the charge result in one
// transaction. A repeated event, or a new event with the same result, is a
// duplicate; a different result for a decided charge is a conflict.
func (repository *Repository) ApplyWebhook(
	ctx context.Context,
	event WebhookEvent,
	reason string,
) (WebhookOutcome, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return WebhookOutcome{}, fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	charge, err := scanCharge(tx.QueryRow(
		ctx,
		`SELECT `+chargeColumns+`
		   FROM pagamento.payment_charges
		  WHERE id = $1::uuid
		    FOR UPDATE`,
		event.ChargeID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return WebhookOutcome{}, ErrChargeNotFound
	}

	if err != nil {
		return WebhookOutcome{}, fmt.Errorf("erro ao consultar cobrança: %w", err)
	}

	if charge.OrderID != event.OrderID {
		return WebhookOutcome{}, fmt.Errorf(
			"%w: cobrança %s não pertence ao pedido %s",
			ErrInvalidWebhook,
			event.ChargeID,
			event.OrderID,
		)
	}

	if charge.Status != ChargePending && charge.Status != event.Status {
		return WebhookOutcome{}, ErrConflictingResult
	}

	tag, err := tx.Exec(
		ctx,
		`INSERT INTO pagamento.webhook_events (external_event_id, charge_id)
		 VALUES ($1, $2::uuid)
		 ON CONFLICT (external_event_id) DO NOTHING`,
		event.EventID,
		event.ChargeID,
	)
	if err != nil {
		return WebhookOutcome{}, fmt.Errorf("erro ao registrar webhook: %w", err)
	}

	if tag.RowsAffected() == 0 || charge.Status == event.Status {
		if err := tx.Commit(ctx); err != nil {
			return WebhookOutcome{}, fmt.Errorf("erro ao confirmar webhook repetido: %w", err)
		}

		return WebhookOutcome{Charge: charge, Duplicate: true}, nil
	}

	charge, err = scanCharge(tx.QueryRow(
		ctx,
		`UPDATE pagamento.payment_charges
		    SET status = $2,
		        processed_at = $3,
		        reason = NULLIF($4, ''),
		        updated_at = NOW()
		  WHERE id = $1::uuid
		  RETURNING `+chargeColumns,
		event.ChargeID,
		event.Status,
		event.OccurredAt,
		reason,
	))
	if err != nil {
		return WebhookOutcome{}, fmt.Errorf("erro ao atualizar cobrança: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return WebhookOutcome{}, fmt.Errorf("erro ao confirmar webhook: %w", err)
	}

	return WebhookOutcome{Charge: charge}, nil
}
