package promotions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
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
// Subscribe inscreve o cliente mantendo o e-mail atual da preferência (ou o
// da conta, na primeira inscrição).
func (repository *Repository) Subscribe(
	ctx context.Context,
	userID string,
	categories []string,
) (bool, error) {
	return repository.SubscribeWithEmail(ctx, userID, categories, "")
}

// SubscribeWithEmail inscreve o cliente nas categorias usando o e-mail
// informado. Trocar só o e-mail também gera evento, com todas as categorias
// ativas, para o MS Promoções atualizar o destinatário.
func (repository *Repository) SubscribeWithEmail(
	ctx context.Context,
	userID string,
	categories []string,
	requestedEmail string,
) (bool, error) {
	if err := validateCategories(categories); err != nil {
		return false, err
	}

	if requestedEmail != "" {
		normalized, err := normalizeEmail(requestedEmail)
		if err != nil {
			return false, err
		}
		requestedEmail = normalized
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

	if requestedEmail != "" {
		email = requestedEmail
	} else if stored, err := storedPreferenceEmail(ctx, tx, userID); err != nil {
		return false, err
	} else if stored != "" {
		email = stored
	}

	changed := make([]string, 0, len(requested))

	for _, category := range requested {
		var savedCategory string

		err = tx.QueryRow(ctx, `
			INSERT INTO gateway.promotion_preferences AS preference
				(user_id, category, active, email)
			VALUES ($1, $2, TRUE, $3)
			ON CONFLICT (user_id, category)
			DO UPDATE SET
				active = TRUE,
				email = EXCLUDED.email,
				updated_at = clock_timestamp()
			WHERE preference.active = FALSE
			   OR preference.email IS DISTINCT FROM EXCLUDED.email
			RETURNING category
		`, userID, category, email).Scan(&savedCategory)

		if errors.Is(err, pgx.ErrNoRows) {
			// Categoria já ativa: não cria outro evento.
			continue
		}
		if err != nil {
			return false, fmt.Errorf("erro ao salvar preferência: %w", err)
		}

		changed = append(changed, savedCategory)
	}

	// As demais categorias ativas passam a usar o mesmo e-mail.
	otherRows, err := tx.Query(ctx, `
		UPDATE gateway.promotion_preferences
		SET email = $2, updated_at = clock_timestamp()
		WHERE user_id = $1
		  AND active = TRUE
		  AND email IS DISTINCT FROM $2
		RETURNING category
	`, userID, email)
	if err != nil {
		return false, fmt.Errorf("erro ao atualizar e-mail das preferências: %w", err)
	}

	for otherRows.Next() {
		var category string
		if err := otherRows.Scan(&category); err != nil {
			otherRows.Close()
			return false, fmt.Errorf("erro ao ler preferência: %w", err)
		}
		if !slices.Contains(changed, category) {
			changed = append(changed, category)
		}
	}
	otherRows.Close()
	if err := otherRows.Err(); err != nil {
		return false, fmt.Errorf("erro ao atualizar e-mail das preferências: %w", err)
	}

	sort.Strings(changed)

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

// storedPreferenceEmail devolve o e-mail da preferência ativa mais recente.
func storedPreferenceEmail(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var stored string

	err := tx.QueryRow(ctx, `
		SELECT email
		FROM gateway.promotion_preferences
		WHERE user_id = $1 AND active = TRUE AND email IS NOT NULL
		ORDER BY updated_at DESC
		LIMIT 1
	`, userID).Scan(&stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("erro ao consultar e-mail da preferência: %w", err)
	}

	return stored, nil
}

// PreferredEmail devolve o e-mail usado nas promoções: o informado na
// inscrição ativa mais recente ou, sem ele, o da conta.
func (repository *Repository) PreferredEmail(ctx context.Context, userID string) (string, error) {
	var email string

	err := repository.pool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT p.email
			 FROM gateway.promotion_preferences p
			 WHERE p.user_id = u.id AND p.active = TRUE AND p.email IS NOT NULL
			 ORDER BY p.updated_at DESC
			 LIMIT 1),
			u.email
		)
		FROM gateway.users u
		WHERE u.id = $1
	`, userID).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("erro ao consultar e-mail das promoções: %w", err)
	}

	return email, nil
}
