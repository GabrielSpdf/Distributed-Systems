package msestoque

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns every query against the estoque schema.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (repository *Repository) Ping(ctx context.Context) error {
	return repository.pool.Ping(ctx)
}

// ListProducts returns the active products that match the filter.
func (repository *Repository) ListProducts(
	ctx context.Context,
	filter ProductFilter,
) ([]Product, error) {
	const query = `
		SELECT
			id,
			name,
			category,
			price::double precision,
			quantity
		FROM estoque.products
		WHERE active = TRUE
		  AND ($1::text = '' OR category = $1::text)
		  AND ($2::boolean IS NULL OR (quantity > 0) = $2::boolean)
		ORDER BY id
	`

	rows, err := repository.pool.Query(
		ctx,
		query,
		filter.Category,
		filter.Available,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar produtos: %w", err)
	}
	defer rows.Close()

	products := make([]Product, 0)

	for rows.Next() {
		var product Product

		if err := rows.Scan(
			&product.ID,
			&product.Name,
			&product.Category,
			&product.Price,
			&product.Quantity,
		); err != nil {
			return nil, fmt.Errorf("erro ao ler produto: %w", err)
		}

		products = append(products, product)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura dos produtos: %w", err)
	}

	return products, nil
}

// Reserve checks every item and, only if all are available, lowers the stock
// and records the reservation in one transaction. An order that already has a
// stored outcome is never processed twice.
func (repository *Repository) Reserve(
	ctx context.Context,
	orderID string,
	items []ReservationItem,
) (ReservationResult, error) {
	if orderID == "" {
		return ReservationResult{}, fmt.Errorf(
			"%w: pedido sem identificador",
			ErrInvalidOrder,
		)
	}

	requested, err := aggregateItems(items)
	if err != nil {
		return ReservationResult{}, err
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	var (
		existingStatus string
		createdAt      time.Time
	)

	err = tx.QueryRow(
		ctx,
		`SELECT status, created_at
		   FROM estoque.stock_reservations
		  WHERE order_id = $1
		    FOR UPDATE`,
		orderID,
	).Scan(&existingStatus, &createdAt)

	alreadyProcessed := err == nil

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ReservationResult{}, fmt.Errorf("erro ao consultar reserva: %w", err)
	}

	if alreadyProcessed && existingStatus != ReservationUnavailable {
		return ReservationResult{
			Status:     existingStatus,
			ReservedAt: createdAt.UTC(),
		}, nil
	}

	available, err := lockProducts(ctx, tx, requested)
	if err != nil {
		return ReservationResult{}, err
	}

	shortfalls := findShortfalls(requested, available)

	if alreadyProcessed {
		return ReservationResult{
			Status:     ReservationUnavailable,
			Shortfalls: shortfalls,
		}, nil
	}

	if len(shortfalls) > 0 {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO estoque.stock_reservations (order_id, status)
			 VALUES ($1, $2)`,
			orderID,
			ReservationUnavailable,
		); err != nil {
			return ReservationResult{}, fmt.Errorf("erro ao registrar indisponibilidade: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return ReservationResult{}, fmt.Errorf("erro ao confirmar indisponibilidade: %w", err)
		}

		return ReservationResult{
			Status:     ReservationUnavailable,
			Shortfalls: shortfalls,
		}, nil
	}

	err = tx.QueryRow(
		ctx,
		`INSERT INTO estoque.stock_reservations (order_id, status)
		 VALUES ($1, $2)
		 RETURNING created_at`,
		orderID,
		ReservationReserved,
	).Scan(&createdAt)
	if err != nil {
		return ReservationResult{}, fmt.Errorf("erro ao registrar reserva: %w", err)
	}

	for _, item := range requested {
		if _, err := tx.Exec(
			ctx,
			`UPDATE estoque.products
			    SET quantity = quantity - $2,
			        updated_at = NOW()
			  WHERE id = $1`,
			item.ProductID,
			item.Quantity,
		); err != nil {
			return ReservationResult{}, fmt.Errorf(
				"erro ao baixar estoque do produto %s: %w",
				item.ProductID,
				err,
			)
		}

		if _, err := tx.Exec(
			ctx,
			`INSERT INTO estoque.stock_reservation_items (order_id, product_id, quantity)
			 VALUES ($1, $2, $3)`,
			orderID,
			item.ProductID,
			item.Quantity,
		); err != nil {
			return ReservationResult{}, fmt.Errorf(
				"erro ao registrar item %s da reserva: %w",
				item.ProductID,
				err,
			)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, fmt.Errorf("erro ao confirmar reserva: %w", err)
	}

	return ReservationResult{
		Status:     ReservationReserved,
		Shortfalls: []Shortfall{},
		ReservedAt: createdAt.UTC(),
	}, nil
}

// lockProducts reads the current quantities and locks those rows until the
// transaction ends, so two orders cannot take the same units.
func lockProducts(
	ctx context.Context,
	tx pgx.Tx,
	requested []ReservationItem,
) (map[string]int, error) {
	ids := make([]string, 0, len(requested))

	for _, item := range requested {
		ids = append(ids, item.ProductID)
	}

	rows, err := tx.Query(
		ctx,
		`SELECT id, quantity
		   FROM estoque.products
		  WHERE id = ANY($1)
		    AND active = TRUE
		  ORDER BY id
		    FOR UPDATE`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao bloquear produtos: %w", err)
	}
	defer rows.Close()

	available := make(map[string]int, len(ids))

	for rows.Next() {
		var (
			id       string
			quantity int
		)

		if err := rows.Scan(&id, &quantity); err != nil {
			return nil, fmt.Errorf("erro ao ler quantidade: %w", err)
		}

		available[id] = quantity
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura das quantidades: %w", err)
	}

	return available, nil
}

// Release gives the reserved units back. When the order has no stored outcome
// yet, it records RELEASED so a late pedido.criado cannot reserve it.
func (repository *Repository) Release(
	ctx context.Context,
	orderID string,
) (string, error) {
	if orderID == "" {
		return "", fmt.Errorf("%w: pedido sem identificador", ErrInvalidOrder)
	}

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	var status string

	err = tx.QueryRow(
		ctx,
		`SELECT status
		   FROM estoque.stock_reservations
		  WHERE order_id = $1
		    FOR UPDATE`,
		orderID,
	).Scan(&status)

	if errors.Is(err, pgx.ErrNoRows) {
		if _, err := tx.Exec(
			ctx,
			`INSERT INTO estoque.stock_reservations (order_id, status)
			 VALUES ($1, $2)`,
			orderID,
			ReservationReleased,
		); err != nil {
			return "", fmt.Errorf("erro ao registrar cancelamento preventivo: %w", err)
		}

		if err := tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("erro ao confirmar cancelamento preventivo: %w", err)
		}

		return ReservationReleased, nil
	}

	if err != nil {
		return "", fmt.Errorf("erro ao consultar reserva: %w", err)
	}

	if status != ReservationReserved {
		return status, nil
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE estoque.products AS product
		    SET quantity = product.quantity + item.quantity,
		        updated_at = NOW()
		   FROM estoque.stock_reservation_items AS item
		  WHERE item.order_id = $1
		    AND product.id = item.product_id`,
		orderID,
	); err != nil {
		return "", fmt.Errorf("erro ao devolver estoque: %w", err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE estoque.stock_reservations
		    SET status = $2,
		        updated_at = NOW()
		  WHERE order_id = $1`,
		orderID,
		ReservationReleased,
	); err != nil {
		return "", fmt.Errorf("erro ao atualizar reserva: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("erro ao confirmar devolução: %w", err)
	}

	return ReservationReleased, nil
}
