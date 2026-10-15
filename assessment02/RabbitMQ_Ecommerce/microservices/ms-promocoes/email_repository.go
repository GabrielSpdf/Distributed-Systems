package mspromocoes

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type EmailRepository struct {
	pool *pgxpool.Pool
}

func NewEmailRepository(pool *pgxpool.Pool) *EmailRepository {
	return &EmailRepository{pool: pool}
}

// QueuePromotionEmails cria envios pendentes, sem enviar mensagens.
func (repository *EmailRepository) QueuePromotionEmails(
	ctx context.Context,
	promotionEventID string,
	promotion events.PromotionPayload,
	from string,
	testRecipient string,
) (int64, error) {
	var identifier pgtype.UUID
	if err := identifier.Scan(promotionEventID); err != nil || !identifier.Valid {
		return 0, fmt.Errorf("identificador da promoção inválido")
	}

	from = strings.TrimSpace(from)
	testRecipient = strings.TrimSpace(testRecipient)

	if _, err := mail.ParseAddress(from); err != nil {
		return 0, fmt.Errorf("remetente inválido")
	}

	address, err := mail.ParseAddress(testRecipient)
	if err != nil || address.Address != testRecipient {
		return 0, fmt.Errorf("destinatário de teste inválido")
	}

	category, err := normalizePromotionCategory(promotion.Category)
	if err != nil {
		return 0, err
	}

	message, err := BuildPromotionEmail(promotion, testRecipient)
	if err != nil {
		return 0, err
	}

	// Uma única instrução grava todos os envios de forma atômica.
	result, err := repository.pool.Exec(ctx, `
		INSERT INTO promocoes.email_jobs (
			promotion_event_id,
			user_id,
			category,
			from_address,
			recipient,
			subject,
			text_body,
			html_body,
			idempotency_key
		)
		SELECT
			$1::uuid,
			interest.user_id,
			$2::varchar(100),
			$3,
			interest.email,
			$5,
			$6,
			$7,
			'promotion/' || ($1::uuid)::text ||
				'/' || interest.user_id::text
		FROM promocoes.promotion_interests AS interest
		WHERE interest.active = TRUE
		  AND interest.category = $2::varchar(100)
		  AND LOWER(interest.email) = LOWER($4::text)
		ON CONFLICT (promotion_event_id, user_id) DO NOTHING
	`,
		promotionEventID,
		category,
		from,
		testRecipient,
		message.Subject,
		message.Text,
		message.HTML,
	)
	if err != nil {
		return 0, fmt.Errorf("erro ao criar envios pendentes: %w", err)
	}

	return result.RowsAffected(), nil
}

// PrepareEmailJob registra o início da janela de tentativas.
// Não envia e-mail nem incrementa o contador de tentativas.
func (repository *EmailRepository) PrepareEmailJob(
	ctx context.Context,
	jobID string,
) (bool, error) {
	result, err := repository.pool.Exec(ctx, `
		UPDATE promocoes.email_jobs
		SET first_attempt_at = COALESCE(
			first_attempt_at,
			clock_timestamp()
		)
		WHERE id = $1::uuid
		  AND status = 'PENDING'
		  AND next_attempt_at <= NOW()
	`, jobID)
	if err != nil {
		return false, fmt.Errorf("erro ao preparar envio: %w", err)
	}

	return result.RowsAffected() == 1, nil
}
