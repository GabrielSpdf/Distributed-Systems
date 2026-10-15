package mspromocoes

import (
	"context"
	"encoding/json"
	"fmt"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

type InterestRepository struct {
	pool *pgxpool.Pool
}

func NewInterestRepository(pool *pgxpool.Pool) *InterestRepository {
	return &InterestRepository{pool: pool}
}

// ApplyInterestEvent retorna false quando o evento já foi processado.
func (repository *InterestRepository) ApplyInterestEvent(
	ctx context.Context,
	envelope events.EventEnvelope,
	routingKey string,
	publicKeys cryptography.PublicKeyRegistry,
) (bool, error) {
	payload, err := ValidateInterestEvent(envelope, routingKey, publicKeys)
	if err != nil {
		return false, err
	}

	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		return false, fmt.Errorf("erro ao serializar envelope: %w", err)
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar aplicação do interesse: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serializa as alterações do mesmo cliente dentro deste serviço.
	_, err = tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))
	`, payload.UserID)
	if err != nil {
		return false, fmt.Errorf("erro ao bloquear interesses do cliente: %w", err)
	}

	result, err := tx.Exec(ctx, `
		INSERT INTO promocoes.interest_inbox
			(event_id, event_type, user_id, envelope, occurred_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (event_id) DO NOTHING
	`,
		envelope.EventID,
		envelope.EventType,
		payload.UserID,
		string(envelopeJSON),
		payload.OccurredAt.UTC(),
	)
	if err != nil {
		return false, fmt.Errorf("erro ao registrar evento na inbox: %w", err)
	}

	if result.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("erro ao finalizar evento repetido: %w", err)
		}
		return false, nil
	}

	active := payload.Action == events.PromotionSubscribe

	for _, category := range payload.Categories {
		_, err = tx.Exec(ctx, `
			INSERT INTO promocoes.promotion_interests
				(user_id, email, category, active)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (user_id, category)
			DO UPDATE SET
				email = EXCLUDED.email,
				active = EXCLUDED.active,
				updated_at = clock_timestamp()
		`, payload.UserID, payload.Email, category, active)
		if err != nil {
			return false, fmt.Errorf("erro ao atualizar interesse: %w", err)
		}

		if !active {
			_, err = tx.Exec(ctx, `
				UPDATE promocoes.email_jobs
				SET status = 'CANCELLED',
				    last_error = 'Interesse cancelado pelo cliente'
				WHERE user_id = $1
				  AND category = $2::varchar(100)
				  AND status = 'PENDING'
			`, payload.UserID, category)
			if err != nil {
				return false, fmt.Errorf(
					"erro ao cancelar envios pendentes: %w", err,
				)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao finalizar aplicação do interesse: %w", err)
	}

	return true, nil
}
