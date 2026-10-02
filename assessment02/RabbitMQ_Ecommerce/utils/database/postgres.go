package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const connectionTimeout = 5 * time.Second

func Connect(
	ctx context.Context,
	databaseURL string,
) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("URL do banco de dados não informada")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao interpretar configuração do PostgreSQL: %w",
			err,
		)
	}

	poolConfig.MinConns = 1
	poolConfig.MaxConns = 10
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.MaxConnLifetime = 30 * time.Minute

	connectionContext, cancel := context.WithTimeout(
		ctx,
		connectionTimeout,
	)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(
		connectionContext,
		poolConfig,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"erro ao criar pool do PostgreSQL: %w",
			err,
		)
	}

	if err := pool.Ping(connectionContext); err != nil {
		pool.Close()

		return nil, fmt.Errorf(
			"erro ao verificar conexão com o PostgreSQL: %w",
			err,
		)
	}

	return pool, nil
}
