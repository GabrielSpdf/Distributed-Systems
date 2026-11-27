package promotions

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrOutboxEventNotFound = errors.New("evento pendente não encontrado")

// PrepareOutboxEnvelope salva o envelope assinado sem publicar a mensagem.
func (repository *Repository) PrepareOutboxEnvelope(
	ctx context.Context,
	eventID string,
	privateKey *rsa.PrivateKey,
) error {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("erro ao iniciar preparação da outbox: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID string
	var payloadJSON []byte
	var storedEnvelope string

	err = tx.QueryRow(ctx, `
		SELECT user_id::text, payload, COALESCE(envelope, '')
		FROM gateway.promotion_outbox
		WHERE event_id = $1 AND status = 'PENDENTE'
		FOR UPDATE
	`, eventID).Scan(&userID, &payloadJSON, &storedEnvelope)

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOutboxEventNotFound
	}
	if err != nil {
		return fmt.Errorf("erro ao consultar evento pendente: %w", err)
	}

	if storedEnvelope != "" {
		// Já preparado: preserva exatamente o conteúdo existente.
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("erro ao finalizar preparação: %w", err)
		}
		return nil
	}

	var payload InterestPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return fmt.Errorf("payload de interesse inválido: %w", err)
	}

	if payload.UserID != userID {
		return fmt.Errorf("usuário do payload difere do usuário da outbox")
	}

	envelope, err := BuildInterestEvent(eventID, payload, privateKey)
	if err != nil {
		return err
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("erro ao serializar envelope: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE gateway.promotion_outbox
		SET envelope = $2
		WHERE event_id = $1
	`, eventID, string(data))
	if err != nil {
		return fmt.Errorf("erro ao salvar envelope: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("erro ao finalizar preparação: %w", err)
	}

	return nil
}

// NextPendingOutboxEventID encontra um candidato.
// A publicação ainda precisará bloquear e revalidar esse registro.
func (repository *Repository) NextPendingOutboxEventID(
	ctx context.Context,
) (string, error) {
	var eventID string

	err := repository.pool.QueryRow(ctx, `
		SELECT candidate.event_id::text
		FROM gateway.promotion_outbox AS candidate
		WHERE candidate.status = 'PENDENTE'
		  AND candidate.next_attempt_at <= NOW()
		  AND NOT EXISTS (
			  SELECT 1
			  FROM gateway.promotion_outbox AS earlier
			  WHERE earlier.user_id = candidate.user_id
			    AND earlier.sequence_id < candidate.sequence_id
			    AND earlier.status <> 'PUBLICADO'
		  )
		ORDER BY candidate.sequence_id
		LIMIT 1
	`).Scan(&eventID)

	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("erro ao selecionar evento da outbox: %w", err)
	}

	return eventID, nil
}

// publish deve retornar nil somente após a confirmação da publicação.
func (repository *Repository) PublishPreparedOutboxEvent(
	ctx context.Context,
	eventID string,
	publish func(context.Context, []byte) error,
) (bool, error) {
	if publish == nil {
		return false, fmt.Errorf("publicador não configurado")
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar publicação: %w", err)
	}
	defer tx.Rollback(ctx)

	var envelope string
	var attempts int

	err = tx.QueryRow(ctx, `
		SELECT candidate.envelope, candidate.attempt_count
		FROM gateway.promotion_outbox AS candidate
		WHERE candidate.event_id = $1
		  AND candidate.status = 'PENDENTE'
		  AND candidate.envelope IS NOT NULL
		  AND candidate.next_attempt_at <= NOW()
		  AND NOT EXISTS (
			  SELECT 1
			  FROM gateway.promotion_outbox AS earlier
			  WHERE earlier.user_id = candidate.user_id
			    AND earlier.sequence_id < candidate.sequence_id
			    AND earlier.status <> 'PUBLICADO'
		  )
		FOR UPDATE OF candidate SKIP LOCKED
	`, eventID).Scan(&envelope, &attempts)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao bloquear evento: %w", err)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	publishErr := publish(attemptCtx, []byte(envelope))
	cancel()

	if ctx.Err() != nil {
		return false, ctx.Err()
	}

	attempts++

	if publishErr == nil {
		_, err = tx.Exec(ctx, `
			UPDATE gateway.promotion_outbox
			SET status = 'PUBLICADO',
			    attempt_count = $2,
			    published_at = clock_timestamp(),
			    last_error = NULL
			WHERE event_id = $1
		`, eventID, attempts)
	} else {
		nextStatus := "PENDENTE"
		if attempts >= 12 {
			nextStatus = "PENDENTE_REVISAO"
		}

		delay := time.Duration(min(60, 5*attempts)) * time.Second
		nextAttempt := time.Now().UTC().Add(delay)

		_, err = tx.Exec(ctx, `
			UPDATE gateway.promotion_outbox
			SET status = $2::varchar,
			    attempt_count = $3,
			    next_attempt_at = $4,
			    last_error = $5
			WHERE event_id = $1
		`, eventID, nextStatus, attempts, nextAttempt, publishErr.Error())
	}

	if err != nil {
		return false, fmt.Errorf("erro ao registrar tentativa: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao finalizar publicação: %w", err)
	}

	if publishErr != nil {
		return true, fmt.Errorf("publicação falhou e tentativa foi registrada: %w", publishErr)
	}

	return true, nil
}
