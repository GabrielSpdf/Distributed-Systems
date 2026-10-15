package mspromocoes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (repository *EmailRepository) ProcessEmailJob(
	ctx context.Context,
	jobID string,
	configuredFrom string,
	testRecipient string,
	send func(context.Context, EmailMessage, string) (string, error),
) (bool, error) {
	if send == nil {
		return false, fmt.Errorf("remetente de e-mails não configurado")
	}

	// Esta gravação é confirmada antes de qualquer chamada externa.
	prepared, err := repository.PrepareEmailJob(ctx, jobID)
	if err != nil || !prepared {
		return false, err
	}

	var userID string
	err = repository.pool.QueryRow(ctx, `
		SELECT user_id::text
		FROM promocoes.email_jobs
		WHERE id = $1
	`, jobID).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	// Mesma ordem de bloqueios usada no cancelamento: cliente, depois envio.
	_, err = tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))
	`, userID)
	if err != nil {
		return false, err
	}

	var message EmailMessage
	var category, savedFrom, idempotencyKey string
	var attempts int
	var windowExpired bool

	err = tx.QueryRow(ctx, `
		SELECT category, from_address, recipient,
		       subject, text_body, html_body,
		       idempotency_key, attempt_count,
		       COALESCE(
		           first_attempt_at <= clock_timestamp() - INTERVAL '23 hours',
		           TRUE
		       )
		FROM promocoes.email_jobs
		WHERE id = $1
		  AND status = 'PENDING'
		  AND next_attempt_at <= NOW()
		FOR UPDATE SKIP LOCKED
	`, jobID).Scan(
		&category, &savedFrom, &message.To,
		&message.Subject, &message.Text, &message.HTML,
		&idempotencyKey, &attempts, &windowExpired,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	finishWithoutSending := func(status, reason string) (bool, error) {
		_, err := tx.Exec(ctx, `
			UPDATE promocoes.email_jobs
			SET status = $2::varchar, last_error = $3::text
			WHERE id = $1
		`, jobID, status, reason)
		if err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return true, nil
	}

	var interested bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM promocoes.promotion_interests
			WHERE user_id = $1
			  AND category = $2::varchar(100)
			  AND active = TRUE
			  AND LOWER(email) = LOWER($3::text)
		)
	`, userID, category, message.To).Scan(&interested)
	if err != nil {
		return false, err
	}
	if !interested {
		return finishWithoutSending("CANCELLED", "Interesse não está mais ativo")
	}

	if savedFrom != strings.TrimSpace(configuredFrom) ||
		!strings.EqualFold(message.To, strings.TrimSpace(testRecipient)) {
		return finishWithoutSending(
			"PENDING_REVIEW", "Configuração de envio difere do conteúdo salvo",
		)
	}

	if windowExpired || attempts >= 12 {
		return finishWithoutSending(
			"PENDING_REVIEW", "Limite de tentativas ou janela de envio excedido",
		)
	}

	attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	providerID, sendErr := send(attemptCtx, message, idempotencyKey)
	cancel()

	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	if sendErr == nil &&
		(strings.TrimSpace(providerID) == "" || len(providerID) > 255) {
		sendErr = fmt.Errorf("provedor respondeu sem identificador válido")
	}

	attempts++
	jobStatus := "SENT"
	logStatus := "SENT"
	var errorValue any
	var providerValue any = providerID

	if sendErr != nil {
		jobStatus = "PENDING"
		logStatus = "FAILED"
		errorValue = sendErr.Error()
		providerValue = nil

		var providerError *ResendHTTPError
		if errors.As(sendErr, &providerError) && !providerError.Temporary() {
			jobStatus = "FAILED"
		} else if attempts >= 12 {
			jobStatus = "PENDING_REVIEW"
		}
	}

	nextAttempt := time.Now().UTC().Add(
		time.Duration(min(60, 5*attempts)) * time.Second,
	)

	_, err = tx.Exec(ctx, `
		UPDATE promocoes.email_jobs
		SET status = $2::varchar,
		    attempt_count = $3,
		    next_attempt_at = $4,
		    last_error = $5::text,
		    provider_message_id = $6::varchar,
		    sent_at = CASE
		        WHEN $2::varchar = 'SENT' THEN clock_timestamp()
		        ELSE NULL
		    END
		WHERE id = $1
	`, jobID, jobStatus, attempts, nextAttempt, errorValue, providerValue)
	if err != nil {
		return false, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO promocoes.email_logs (
			job_id, attempt_number, user_id,
			email, category, provider_message_id,
			status, error_message
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, jobID, attempts, userID, message.To, category,
		providerValue, logStatus, errorValue)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}

	if sendErr != nil {
		return true, fmt.Errorf("falha de envio registrada: %w", sendErr)
	}
	return true, nil
}
