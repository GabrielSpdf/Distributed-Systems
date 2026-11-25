package mspromocoes

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

// ProcessNextEmailJob procura um envio pendente e tenta processá-lo.
func (repository *EmailRepository) ProcessNextEmailJob(
	ctx context.Context,
	from string,
	testRecipient string,
	send func(context.Context, EmailMessage, string) (string, error),
) (bool, error) {
	if send == nil {
		return false, fmt.Errorf("remetente de e-mails não configurado")
	}

	var jobID string
	err := repository.pool.QueryRow(ctx, `
		SELECT id::text
		FROM promocoes.email_jobs
		WHERE status = 'PENDING'
		  AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at, created_at, id
		LIMIT 1
	`).Scan(&jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao buscar envio pendente: %w", err)
	}

	return repository.ProcessEmailJob(
		ctx, jobID, from, testRecipient, send,
	)
}

// RunEmailWorker executa os envios até o contexto ser cancelado.
func (repository *EmailRepository) RunEmailWorker(
	ctx context.Context,
	from string,
	testRecipient string,
	send func(context.Context, EmailMessage, string) (string, error),
) error {
	if send == nil {
		return fmt.Errorf("remetente de e-mails não configurado")
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		attemptCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		handled, err := repository.ProcessNextEmailJob(
			attemptCtx, from, testRecipient, send,
		)
		cancel()

		if ctx.Err() != nil {
			return nil
		}

		delay := time.Second

		if err != nil {
			log.Printf("[ERRO] Processamento de e-mail: %v", err)
			delay = 5 * time.Second
		} else if handled {
			// Processado também pode significar cancelado ou em revisão.
			continue
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
