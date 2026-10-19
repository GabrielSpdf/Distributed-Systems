package mspagamento

import (
	"context"
	"errors"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

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
	result_published_at IS NOT NULL,
	COALESCE(checkout_envelope, ''),
	COALESCE(result_envelope, '')
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
		&charge.CheckoutEnvelope,
		&charge.ResultEnvelope,
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
	if !sameChargeBusinessData(stored, charge) || stored.ID != charge.ID ||
		stored.CheckoutURL != charge.CheckoutURL ||
		!stored.ExpiresAt.Truncate(time.Microsecond).Equal(charge.ExpiresAt.Truncate(time.Microsecond)) {
		return Charge{}, fmt.Errorf("%w: pedido %s", ErrConflictingCharge, charge.OrderID)
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

	// Uma decisão posterior ao prazo não vale, mesmo que a expiração ainda
	// não tenha sido gravada.
	if charge.Status == ChargePending && !charge.ExpiresAt.IsZero() && event.OccurredAt.After(charge.ExpiresAt) {
		return WebhookOutcome{}, ErrCheckoutExpired
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

	if tag.RowsAffected() == 0 {
		// Um event_id já gravado só é repetição se for da mesma cobrança.
		var ownerChargeID string

		if err := tx.QueryRow(
			ctx,
			`SELECT charge_id::text
			   FROM pagamento.webhook_events
			  WHERE external_event_id = $1`,
			event.EventID,
		).Scan(&ownerChargeID); err != nil {
			return WebhookOutcome{}, fmt.Errorf("erro ao consultar webhook repetido: %w", err)
		}

		if ownerChargeID != charge.ID {
			return WebhookOutcome{}, ErrWebhookEventReused
		}
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

func (repository *Repository) SaveCheckoutEnvelope(ctx context.Context, chargeID string, envelope string) (string, error) {
	return repository.saveEnvelope(ctx, chargeID, "checkout_envelope", envelope)
}

func (repository *Repository) SaveResultEnvelope(ctx context.Context, chargeID string, envelope string) (string, error) {
	return repository.saveEnvelope(ctx, chargeID, "result_envelope", envelope)
}

// saveEnvelope stores a signed event once: the first envelope saved wins, so
// every resend uses the same event_id. Column receives only the two names
// above, never user input.
func (repository *Repository) saveEnvelope(
	ctx context.Context,
	chargeID string,
	column string,
	envelope string,
) (string, error) {
	var stored string

	err := repository.pool.QueryRow(
		ctx,
		// COALESCE mantém o envelope já gravado; um segundo gravador concorrente
		// espera o lock, relê a linha e recebe o envelope do primeiro.
		`UPDATE pagamento.payment_charges
		    SET `+column+` = COALESCE(`+column+`, $2),
		        updated_at = CASE WHEN `+column+` IS NULL THEN NOW() ELSE updated_at END
		  WHERE id = $1::uuid
		  RETURNING `+column,
		chargeID,
		envelope,
	).Scan(&stored)
	if err != nil {
		return "", fmt.Errorf("erro ao gravar %s da cobrança %s: %w", column, chargeID, err)
	}

	return stored, nil
}

// ListPendingCheckouts returns charges whose checkout event was not confirmed
// by the broker.
func (repository *Repository) ListPendingCheckouts(
	ctx context.Context,
	minimumAge time.Duration,
	limit int,
) ([]Charge, error) {
	return repository.listCharges(ctx, `
		checkout_published_at IS NULL
	`, minimumAge, limit)
}

// ListPendingResults returns decided charges whose result event was not
// confirmed by the broker. The checkout goes first, so it must be published.
func (repository *Repository) ListPendingResults(
	ctx context.Context,
	minimumAge time.Duration,
	limit int,
) ([]Charge, error) {
	return repository.listCharges(ctx, `
		status <> 'PENDING'
		AND checkout_published_at IS NOT NULL
		AND result_published_at IS NULL
	`, minimumAge, limit)
}

// listCharges receives only the fixed conditions above, never user input.
func (repository *Repository) listCharges(
	ctx context.Context,
	condition string,
	minimumAge time.Duration,
	limit int,
) ([]Charge, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+chargeColumns+`
		   FROM pagamento.payment_charges
		  WHERE `+condition+`
		    AND updated_at <= NOW() - $1::interval
		  ORDER BY updated_at
		  LIMIT $2`,
		minimumAge,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar cobranças pendentes: %w", err)
	}
	defer rows.Close()

	charges := make([]Charge, 0)

	for rows.Next() {
		charge, err := scanCharge(rows)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler cobrança pendente: %w", err)
		}
		charges = append(charges, charge)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura das cobranças pendentes: %w", err)
	}

	return charges, nil
}

// ExpireNextCharge marks one overdue PENDING charge as EXPIRED. The row lock
// makes the expiry and a concurrent webhook exclusive: whichever locks the
// charge first decides it. grace gives in-flight webhooks decided before the
// deadline a moment to arrive.
func (repository *Repository) ExpireNextCharge(
	ctx context.Context,
	grace time.Duration,
) (Charge, bool, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Charge{}, false, fmt.Errorf("erro ao iniciar expiração: %w", err)
	}
	defer tx.Rollback(ctx)

	var chargeID string

	err = tx.QueryRow(
		ctx,
		`SELECT id::text
		   FROM pagamento.payment_charges
		  WHERE status = $1
		    AND expires_at <= NOW() - $2::interval
		  ORDER BY expires_at, id
		  LIMIT 1
		    FOR UPDATE SKIP LOCKED`,
		ChargePending,
		grace,
	).Scan(&chargeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Charge{}, false, nil
	}

	if err != nil {
		return Charge{}, false, fmt.Errorf("erro ao selecionar cobrança vencida: %w", err)
	}

	charge, err := scanCharge(tx.QueryRow(
		ctx,
		`UPDATE pagamento.payment_charges
		    SET status = $2,
		        processed_at = NOW(),
		        reason = $3,
		        updated_at = NOW()
		  WHERE id = $1::uuid
		  RETURNING `+chargeColumns,
		chargeID,
		ChargeExpired,
		events.ReasonCheckoutExpired,
	))
	if err != nil {
		return Charge{}, false, fmt.Errorf("erro ao expirar cobrança: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Charge{}, false, fmt.Errorf("erro ao confirmar expiração: %w", err)
	}

	return charge, true, nil
}
