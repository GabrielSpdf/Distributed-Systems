package orders

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrOrderNotFound = errors.New("pedido não encontrado")

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

func (repository *Repository) Create(
	ctx context.Context,
	userID string,
	items []OrderItem,
	totalCents int64,
) (Order, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Order{}, fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	var sequence int64

	err = tx.QueryRow(
		ctx,
		"SELECT nextval('gateway.order_display_id_seq')",
	).Scan(&sequence)
	if err != nil {
		return Order{}, fmt.Errorf("erro ao gerar número do pedido: %w", err)
	}

	displayID := fmt.Sprintf("PED-%03d", sequence)

	totalValue := fmt.Sprintf(
		"%d.%02d",
		totalCents/100,
		totalCents%100,
	)

	order := Order{
		UserID:   userID,
		ID:       displayID,
		Status:   StatusPending,
		Items:    items,
		Total:    float64(totalCents) / 100,
		Currency: "BRL",
		Links:    make(map[string]Link),
	}

	const insertOrder = `
		INSERT INTO gateway.orders (
			display_id,
			user_id,
			status,
			total
		)
		VALUES ($1, $2, $3, $4::numeric)
		RETURNING id, created_at, updated_at
	`

	err = tx.QueryRow(
		ctx,
		insertOrder,
		displayID,
		userID,
		StatusPending,
		totalValue,
	).Scan(
		&order.DatabaseID,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		return Order{}, fmt.Errorf("erro ao salvar pedido: %w", err)
	}

	const insertItem = `
		INSERT INTO gateway.order_items (
			order_id,
			product_id,
			product_name,
			quantity,
			unit_price
		)
		VALUES ($1, $2, $3, $4, $5::numeric)
	`

	for _, item := range items {
		_, err = tx.Exec(
			ctx,
			insertItem,
			order.DatabaseID,
			item.ProductID,
			item.ProductName,
			item.Quantity,
			fmt.Sprintf("%.2f", item.UnitPrice),
		)
		if err != nil {
			return Order{}, fmt.Errorf("erro ao salvar item: %w", err)
		}
	}

	const insertHistory = `
		INSERT INTO gateway.order_status_history (
			order_id,
			status
		)
		VALUES ($1, $2)
	`

	_, err = tx.Exec(
		ctx,
		insertHistory,
		order.DatabaseID,
		StatusPending,
	)
	if err != nil {
		return Order{}, fmt.Errorf("erro ao salvar histórico: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("erro ao confirmar pedido: %w", err)
	}

	order.CreatedAt = order.CreatedAt.UTC()
	order.UpdatedAt = order.UpdatedAt.UTC()

	return order, nil
}

func (repository *Repository) ListByUser(
	ctx context.Context,
	userID string,
) ([]Order, error) {
	const query = `
		SELECT
			id,
			display_id,
			status,
			total::double precision,
			COALESCE(checkout_url, ''),
			created_at,
			updated_at
		FROM gateway.orders
		WHERE user_id = $1
		  AND is_deleted = FALSE
		ORDER BY created_at DESC, display_id DESC
	`

	rows, err := repository.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar pedidos: %w", err)
	}
	defer rows.Close()

	orders := make([]Order, 0)

	for rows.Next() {
		order := Order{
			UserID:   userID,
			Currency: "BRL",
			Items:    make([]OrderItem, 0),
			Links:    make(map[string]Link),
		}

		err := rows.Scan(
			&order.DatabaseID,
			&order.ID,
			&order.Status,
			&order.Total,
			&order.CheckoutURL,
			&order.CreatedAt,
			&order.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler pedido: %w", err)
		}

		order.CreatedAt = order.CreatedAt.UTC()
		order.UpdatedAt = order.UpdatedAt.UTC()

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura dos pedidos: %w", err)
	}

	rows.Close()

	for index := range orders {
		items, err := repository.listItems(
			ctx,
			orders[index].DatabaseID,
		)
		if err != nil {
			return nil, err
		}

		orders[index].Items = items
	}

	return orders, nil
}

func (repository *Repository) listItems(
	ctx context.Context,
	orderID string,
) ([]OrderItem, error) {
	const query = `
		SELECT
			product_id,
			product_name,
			quantity,
			unit_price::double precision
		FROM gateway.order_items
		WHERE order_id = $1
		ORDER BY product_id
	`

	rows, err := repository.pool.Query(ctx, query, orderID)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar itens: %w", err)
	}
	defer rows.Close()

	items := make([]OrderItem, 0)

	for rows.Next() {
		var item OrderItem

		if err := rows.Scan(
			&item.ProductID,
			&item.ProductName,
			&item.Quantity,
			&item.UnitPrice,
		); err != nil {
			return nil, fmt.Errorf("erro ao ler item: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura dos itens: %w", err)
	}

	return items, nil
}

func (repository *Repository) FindByID(
	ctx context.Context,
	userID string,
	displayID string,
) (Order, error) {
	const query = `
		SELECT
			id,
			display_id,
			status,
			total::double precision,
			COALESCE(checkout_url, ''),
			created_at,
			updated_at
		FROM gateway.orders
		WHERE display_id = $1
		  AND user_id = $2
		  AND is_deleted = FALSE
	`

	order := Order{
		UserID:   userID,
		Currency: "BRL",
		Links:    make(map[string]Link),
	}

	err := repository.pool.QueryRow(
		ctx,
		query,
		displayID,
		userID,
	).Scan(
		&order.DatabaseID,
		&order.ID,
		&order.Status,
		&order.Total,
		&order.CheckoutURL,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}

	if err != nil {
		return Order{}, fmt.Errorf("erro ao consultar pedido: %w", err)
	}

	items, err := repository.listItems(
		ctx,
		order.DatabaseID,
	)
	if err != nil {
		return Order{}, err
	}

	order.Items = items
	order.CreatedAt = order.CreatedAt.UTC()
	order.UpdatedAt = order.UpdatedAt.UTC()

	return order, nil
}
