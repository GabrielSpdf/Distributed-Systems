package promotions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("usuário não encontrado")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// Subscribe retorna true quando alguma preferência foi alterada.
func (repository *Repository) Subscribe(
	ctx context.Context,
	userID string,
	categories []string,
) (bool, error) {
	if err := validateCategories(categories); err != nil {
		return false, err
	}

	// Ordena uma cópia sem modificar a entrada.
	requested := append([]string(nil), categories...)
	sort.Strings(requested)

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	// Serializa mudanças do mesmo usuário e obtém o e-mail do banco.
	var email string
	err = tx.QueryRow(ctx, `
		SELECT email
		FROM gateway.users
		WHERE id = $1
		FOR UPDATE
	`, userID).Scan(&email)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("erro ao consultar usuário: %w", err)
	}

	changed := make([]string, 0, len(requested))

	for _, category := range requested {
		var savedCategory string

		err = tx.QueryRow(ctx, `
			INSERT INTO gateway.promotion_preferences AS preference
				(user_id, category, active)
			VALUES ($1, $2, TRUE)
			ON CONFLICT (user_id, category)
			DO UPDATE SET
				active = TRUE,
				updated_at = clock_timestamp()
			WHERE preference.active = FALSE
			RETURNING category
		`, userID, category).Scan(&savedCategory)

		if errors.Is(err, pgx.ErrNoRows) {
			// Categoria já ativa: não cria outro evento.
			continue
		}
		if err != nil {
			return false, fmt.Errorf("erro ao salvar preferência: %w", err)
		}

		changed = append(changed, savedCategory)
	}

	if len(changed) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("erro ao finalizar transação: %w", err)
		}
		return false, nil
	}

	payload, err := json.Marshal(InterestPayload{
		UserID:     userID,
		Email:      email,
		Action:     ActionSubscribe,
		Categories: changed,
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		return false, fmt.Errorf("erro ao montar evento de interesse: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.promotion_outbox (user_id, payload)
		VALUES ($1, $2::jsonb)
	`, userID, string(payload))
	if err != nil {
		return false, fmt.Errorf("erro ao salvar evento na outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao finalizar transação: %w", err)
	}

	return true, nil
}

func (repository *Repository) ListActive(
	ctx context.Context,
	userID string,
) ([]string, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT category
		FROM gateway.promotion_preferences
		WHERE user_id = $1 AND active = TRUE
		ORDER BY category
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar preferências: %w", err)
	}
	defer rows.Close()

	categories := make([]string, 0)

	for rows.Next() {
		var category string
		if err := rows.Scan(&category); err != nil {
			return nil, fmt.Errorf("erro ao ler preferência: %w", err)
		}
		categories = append(categories, category)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro ao consultar preferências: %w", err)
	}

	return categories, nil
}

func (repository *Repository) Unsubscribe(
	ctx context.Context,
	userID string,
	category string,
) (bool, error) {
	if err := validateCategories([]string{category}); err != nil {
		return false, err
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	var email string
	err = tx.QueryRow(ctx, `
		SELECT email
		FROM gateway.users
		WHERE id = $1
		FOR UPDATE
	`, userID).Scan(&email)

	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("erro ao consultar usuário: %w", err)
	}

	result, err := tx.Exec(ctx, `
		UPDATE gateway.promotion_preferences
		SET active = FALSE, updated_at = clock_timestamp()
		WHERE user_id = $1 AND category = $2 AND active = TRUE
	`, userID, category)
	if err != nil {
		return false, fmt.Errorf("erro ao cancelar preferência: %w", err)
	}

	if result.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("erro ao finalizar transação: %w", err)
		}
		return false, nil
	}

	payload, err := json.Marshal(InterestPayload{
		UserID:     userID,
		Email:      email,
		Action:     ActionUnsubscribe,
		Categories: []string{category},
		OccurredAt: time.Now().UTC(),
	})
	if err != nil {
		return false, fmt.Errorf("erro ao montar evento de interesse: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.promotion_outbox (user_id, payload)
		VALUES ($1, $2::jsonb)
	`, userID, string(payload))
	if err != nil {
		return false, fmt.Errorf("erro ao salvar evento na outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao finalizar transação: %w", err)
	}

	return true, nil
}
