package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailAlreadyUsed = errors.New("e-mail já cadastrado")
	ErrUserNotFound     = errors.New("usuário não encontrado")
)

type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{
		pool: pool,
	}
}

func (repository *UserRepository) Create(
	ctx context.Context,
	name string,
	email string,
	passwordHash string,
) (User, error) {
	email = normalizeEmail(email)

	const query = `
		INSERT INTO gateway.users (
			name,
			email,
			password_hash
		)
		VALUES ($1, $2, $3)
		RETURNING
			id,
			name,
			email,
			password_hash,
			created_at
	`

	var user User

	err := repository.pool.QueryRow(
		ctx,
		query,
		name,
		email,
		passwordHash,
	).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) &&
			postgresError.Code == "23505" {
			return User{}, ErrEmailAlreadyUsed
		}

		return User{}, fmt.Errorf("erro ao cadastrar usuário: %w", err)
	}

	return user, nil
}

func (repository *UserRepository) FindByEmail(
	ctx context.Context,
	email string,
) (User, error) {
	email = normalizeEmail(email)

	const query = `
		SELECT
			id,
			name,
			email,
			password_hash,
			created_at
		FROM gateway.users
		WHERE email = $1
	`

	return repository.findOne(ctx, query, email)
}

func (repository *UserRepository) FindByID(
	ctx context.Context,
	userID string,
) (User, error) {
	const query = `
		SELECT
			id,
			name,
			email,
			password_hash,
			created_at
		FROM gateway.users
		WHERE id = $1
	`

	return repository.findOne(ctx, query, userID)
}

func (repository *UserRepository) findOne(
	ctx context.Context,
	query string,
	argument any,
) (User, error) {
	var user User

	err := repository.pool.QueryRow(
		ctx,
		query,
		argument,
	).Scan(
		&user.ID,
		&user.Name,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		return User{}, fmt.Errorf("erro ao consultar usuário: %w", err)
	}

	return user, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
