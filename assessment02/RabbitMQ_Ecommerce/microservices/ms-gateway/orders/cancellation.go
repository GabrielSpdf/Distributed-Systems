package orders

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5"
)

var ErrCancellationConflict = errors.New(
	"cancelamento permitido somente antes de preparar o pedido para envio",
)

// Cancel retorna true quando cancelou e false para repetição já cancelada.
// A retirada da criação não preparada disputa o mesmo lock da outbox que
// PrepareOutboxEnvelope. Após persistir um envelope, a entrega já pode ter
// ocorrido, mesmo sem confirmação ou atualização do status do pedido.
func (repository *Repository) Cancel(
	ctx context.Context,
	userID string,
	displayID string,
) (bool, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar cancelamento: %w", err)
	}
	defer tx.Rollback(ctx)

	var databaseID, currentStatus string
	err = tx.QueryRow(ctx, `
		SELECT id, status
		FROM gateway.orders
		WHERE display_id = $1
		  AND user_id = $2
		  AND is_deleted = FALSE
		FOR UPDATE
	`, displayID, userID).Scan(&databaseID, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrOrderNotFound
	}
	if err != nil {
		return false, fmt.Errorf("erro ao consultar pedido: %w", err)
	}

	if currentStatus == StatusCancelled {
		return false, nil
	}

	if currentStatus != StatusPending {
		return false, ErrCancellationConflict
	}

	// Retira somente a intenção ainda não assinada. Se o publicador vencer,
	// o DELETE espera o lock e reavalia envelope IS NULL: não cancela.
	// Sem linha (pedido antigo), não há prova segura de que nunca foi enviado.
	withdrawn, err := tx.Exec(ctx, `
		DELETE FROM gateway.event_outbox
		WHERE order_id = $1 AND event_type = $2
		  AND status = 'PENDENTE' AND envelope IS NULL
	`, databaseID, events.OrderCreated)
	if err != nil {
		return false, fmt.Errorf("erro ao retirar criação não enviada: %w", err)
	}
	if withdrawn.RowsAffected() != 1 {
		return false, ErrCancellationConflict
	}

	var cancelledAt time.Time
	err = tx.QueryRow(ctx, `
		UPDATE gateway.orders
		SET status = $2, updated_at = clock_timestamp()
		WHERE id = $1
		RETURNING updated_at
	`, databaseID, StatusCancelled).Scan(&cancelledAt)
	if err != nil {
		return false, fmt.Errorf("erro ao cancelar pedido: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.order_status_history (
			order_id, status, created_at
		)
		VALUES ($1, $2, $3)
	`, databaseID, StatusCancelled, cancelledAt)
	if err != nil {
		return false, fmt.Errorf("erro ao registrar cancelamento: %w", err)
	}

	err = repository.enqueueOrderDeletedTx(
		ctx,
		tx,
		databaseID,
		events.WebOrderDeletedPayload{
			OrderID:     displayID,
			CustomerID:  userID,
			Reason:      events.ReasonCustomerCancelled,
			CancelledAt: cancelledAt.UTC(),
		},
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao confirmar cancelamento: %w", err)
	}

	repository.notifyOrderStatus(
		userID,
		displayID,
		StatusCancelled,
		"",
	)

	return true, nil
}

func (repository *Repository) enqueueOrderDeletedTx(
	ctx context.Context,
	tx pgx.Tx,
	databaseID string,
	payload events.WebOrderDeletedPayload,
) error {
	if err := ValidateOrderDeletedPayload(payload); err != nil {
		return fmt.Errorf("compensação inválida: %w", err)
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar compensação: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.event_outbox (
			order_id, event_type, payload
		)
		VALUES ($1, $2, $3::jsonb)
	`, databaseID, events.OrderDeleted, string(data))
	if err != nil {
		return fmt.Errorf("erro ao guardar compensação: %w", err)
	}

	return nil
}

var ErrOutboxNotPending = errors.New(
	"evento da outbox inexistente ou não pendente",
)

func (repository *Repository) PrepareOutboxEnvelope(
	ctx context.Context,
	eventID string,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	if privateKey == nil {
		return events.EventEnvelope{}, fmt.Errorf("chave privada não configurada")
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao iniciar preparação do envelope: %w", err,
		)
	}
	defer tx.Rollback(ctx)

	var storedEventID, eventType string
	var payloadData, envelopeData []byte
	var createdAt time.Time

	err = tx.QueryRow(ctx, `
		SELECT event_id::text, event_type, payload, envelope, created_at
		FROM gateway.event_outbox
		WHERE event_id = $1 AND status = 'PENDENTE'
		FOR UPDATE
	`, eventID).Scan(
		&storedEventID,
		&eventType,
		&payloadData,
		&envelopeData,
		&createdAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return events.EventEnvelope{}, ErrOutboxNotPending
	}
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao consultar outbox: %w", err,
		)
	}

	if eventType != events.OrderDeleted && eventType != events.OrderCreated {
		return events.EventEnvelope{}, fmt.Errorf(
			"tipo de evento não suportado na outbox: %s", eventType,
		)
	}

	// Reutiliza o envelope que já foi assinado e persistido.
	if len(envelopeData) > 0 {
		var envelope events.EventEnvelope
		if err := json.Unmarshal(envelopeData, &envelope); err != nil {
			return events.EventEnvelope{}, fmt.Errorf(
				"envelope armazenado inválido: %w", err,
			)
		}
		if envelope.EventID != storedEventID ||
			envelope.EventType != eventType ||
			envelope.Producer != events.ProducerPrincipal {
			return events.EventEnvelope{}, fmt.Errorf(
				"envelope armazenado inconsistente",
			)
		}
		return envelope, nil
	}

	var envelope events.EventEnvelope
	switch eventType {
	case events.OrderCreated:
		var payload events.WebOrderCreatedPayload
		if err := json.Unmarshal(payloadData, &payload); err != nil {
			return events.EventEnvelope{}, fmt.Errorf("payload de criação inválido: %w", err)
		}
		envelope, err = BuildStoredOrderCreatedEvent(storedEventID, payload, createdAt, privateKey)
	case events.OrderDeleted:
		var payload events.WebOrderDeletedPayload
		if err := json.Unmarshal(payloadData, &payload); err != nil {
			return events.EventEnvelope{}, fmt.Errorf("payload de compensação inválido: %w", err)
		}
		envelope, err = BuildOrderDeletedEvent(storedEventID, payload, privateKey)
	}
	if err != nil {
		return events.EventEnvelope{}, err
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return events.EventEnvelope{}, err
	}

	_, err = tx.Exec(ctx, `
		UPDATE gateway.event_outbox
		SET envelope = $2::text
		WHERE event_id = $1
	`, storedEventID, string(data))
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao guardar envelope assinado: %w", err,
		)
	}

	if err := tx.Commit(ctx); err != nil {
		return events.EventEnvelope{}, fmt.Errorf(
			"erro ao confirmar envelope: %w", err,
		)
	}

	return envelope, nil
}

// Chamar somente depois da confirmação positiva do RabbitMQ.
func (repository *Repository) MarkOutboxPublished(
	ctx context.Context,
	eventID string,
) error {
	result, err := repository.pool.Exec(ctx, `
		UPDATE gateway.event_outbox
		SET status = 'PUBLICADO',
		    attempt_count = attempt_count + 1,
		    published_at = clock_timestamp(),
		    last_error = NULL
		WHERE event_id = $1
		  AND status = 'PENDENTE'
		  AND envelope IS NOT NULL
	`, eventID)
	if err != nil {
		return fmt.Errorf("erro ao registrar publicação: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrOutboxNotPending
	}
	return nil
}

// Mantém o envelope assinado para reutilizá-lo na próxima tentativa.
func (repository *Repository) ScheduleOutboxRetry(
	ctx context.Context,
	eventID string,
	publicationError error,
) error {
	if publicationError == nil {
		return fmt.Errorf("erro da publicação não informado")
	}

	result, err := repository.pool.Exec(ctx, `
		UPDATE gateway.event_outbox
		SET attempt_count = attempt_count + 1,
		    status = CASE
		        WHEN attempt_count + 1 >= 12
		            THEN 'PENDENTE_REVISAO'
		        ELSE 'PENDENTE'
		    END,
		    next_attempt_at =
		        clock_timestamp() + INTERVAL '10 seconds',
		    last_error = $2::text
		WHERE event_id = $1
		  AND status = 'PENDENTE'
	`, eventID, publicationError.Error())
	if err != nil {
		return fmt.Errorf("erro ao reagendar publicação: %w", err)
	}
	if result.RowsAffected() != 1 {
		return ErrOutboxNotPending
	}
	return nil
}

// Reserva temporariamente um evento pronto para publicação.
// Retorna ID vazio quando não há trabalho disponível.
func (repository *Repository) ClaimNextOutboxEvent(
	ctx context.Context,
) (string, error) {
	var eventID string

	err := repository.pool.QueryRow(ctx, `
		WITH candidate AS (
			SELECT event_id
			FROM gateway.event_outbox
			WHERE status = 'PENDENTE'
			  AND next_attempt_at <= clock_timestamp()
			ORDER BY next_attempt_at, created_at, event_id
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE gateway.event_outbox AS outbox
		SET next_attempt_at =
		    clock_timestamp() + INTERVAL '60 seconds'
		FROM candidate
		WHERE outbox.event_id = candidate.event_id
		RETURNING outbox.event_id::text
	`).Scan(&eventID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("erro ao selecionar evento da outbox: %w", err)
	}

	return eventID, nil
}

// Retorna true quando encontrou um evento para tentar processar.
func (repository *Repository) ProcessNextOutboxEvent(
	ctx context.Context,
	privateKey *rsa.PrivateKey,
	publish func(context.Context, events.EventEnvelope) error,
) (bool, error) {
	if privateKey == nil || publish == nil {
		return false, fmt.Errorf("publicador ou chave não configurados")
	}

	attemptCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	eventID, err := repository.ClaimNextOutboxEvent(attemptCtx)
	if err != nil {
		return false, err
	}
	if eventID == "" {
		return false, nil
	}

	envelope, publicationError := repository.PrepareOutboxEnvelope(
		attemptCtx, eventID, privateKey,
	)

	if publicationError == nil {
		publicationError = cryptography.VerifyEnvelope(
			envelope, &privateKey.PublicKey,
		)
	}
	if publicationError == nil {
		publicationError = publish(attemptCtx, envelope)
	}

	// Se o gateway estiver encerrando, a reserva expira e permite recuperação.
	if ctx.Err() != nil {
		return true, ctx.Err()
	}

	// Usa outro prazo para registrar o resultado mesmo se a tentativa expirou.
	resultCtx, cancelResult := context.WithTimeout(ctx, 5*time.Second)
	defer cancelResult()

	if publicationError != nil {
		if err := repository.ScheduleOutboxRetry(
			resultCtx, eventID, publicationError,
		); err != nil {
			return true, fmt.Errorf(
				"tentativa falhou (%v) e não foi reagendada: %w",
				publicationError, err,
			)
		}
		return true, fmt.Errorf(
			"evento %s reagendado: %w", eventID, publicationError,
		)
	}

	if err := repository.MarkOutboxPublished(resultCtx, eventID); err != nil {
		return true, fmt.Errorf(
			"evento %s confirmado, mas não registrado no banco: %w",
			eventID, err,
		)
	}

	return true, nil
}
