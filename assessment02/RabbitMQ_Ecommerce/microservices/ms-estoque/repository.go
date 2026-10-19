package msestoque

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
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
// ReserveOrder é o caminho dos eventos: guarda e compara o pedido completo,
// sem usar event_id, assinatura ou ordem dos itens como identidade de negócio.
func (repository *Repository) ReserveOrder(ctx context.Context, payload events.WebOrderCreatedPayload) (ReservationResult, error) {
	if err := events.ValidateWebOrderCreated(payload); err != nil {
		return ReservationResult{}, fmt.Errorf("%w: %v", ErrInvalidOrder, err)
	}
	payload.Items = append([]events.WebOrderItem(nil), payload.Items...)
	sort.Slice(payload.Items, func(i, j int) bool { return payload.Items[i].ProductID < payload.Items[j].ProductID })
	items := make([]ReservationItem, 0, len(payload.Items))
	for i := range payload.Items {
		cents, _ := events.PositiveMoneyCents(payload.Items[i].UnitPrice)
		payload.Items[i].UnitPrice = float64(cents) / 100
		items = append(items, ReservationItem{ProductID: payload.Items[i].ProductID, Quantity: payload.Items[i].Quantity})
	}
	cents, _ := events.PositiveMoneyCents(payload.Total)
	payload.Total = float64(cents) / 100
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ReservationResult{}, err
	}
	snapshot := string(encoded)
	return repository.reserve(ctx, payload.OrderID, items, &snapshot)
}

// Reserve mantém a operação de baixo nível usada nos testes de estoque.
// Eventos devem chamar ReserveOrder para validar também cliente e valores.
func (repository *Repository) Reserve(
	ctx context.Context,
	orderID string,
	items []ReservationItem,
) (ReservationResult, error) {
	return repository.reserve(ctx, orderID, items, nil)
}

func (repository *Repository) reserve(
	ctx context.Context,
	orderID string,
	items []ReservationItem,
	snapshot *string,
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
		existingStatus   string
		createdAt        time.Time
		storedShortfalls *string
		storedEnvelope   *string
		published        bool
		sameRequest      bool
		missingRequest   bool
	)

	err = tx.QueryRow(
		ctx,
		`SELECT status, created_at, shortfalls, result_envelope,
		        result_published_at IS NOT NULL,
		        COALESCE($2::jsonb IS NULL OR request_payload = $2::jsonb, FALSE),
		        request_payload IS NULL
		   FROM estoque.stock_reservations
		  WHERE order_id = $1
		    FOR UPDATE`,
		orderID,
		snapshot,
	).Scan(&existingStatus, &createdAt, &storedShortfalls, &storedEnvelope, &published, &sameRequest, &missingRequest)

	alreadyProcessed := err == nil

	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ReservationResult{}, fmt.Errorf("erro ao consultar reserva: %w", err)
	}

	if alreadyProcessed {
		if snapshot == nil && !missingRequest {
			return ReservationResult{}, fmt.Errorf("%w: use ReserveOrder para comparar o pedido original", ErrInvalidOrder)
		}
		// Cancelamento preventivo sem criação conhecida é um tombstone:
		// qualquer criação tardia continua sem reservar nada.
		if snapshot != nil && !(existingStatus == ReservationReleased && missingRequest) && !sameRequest {
			return ReservationResult{}, fmt.Errorf("%w: pedido %s com dados divergentes ou original ausente", ErrInvalidOrder, orderID)
		}
		stored := ReservationResult{
			Status:     existingStatus,
			ReservedAt: createdAt.UTC(),
			Published:  published,
		}

		if storedEnvelope != nil {
			stored.Envelope = *storedEnvelope
		}

		if existingStatus != ReservationUnavailable {
			return stored, nil
		}

		// O resultado original nunca é recalculado: as faltas gravadas valem
		// mesmo que o estoque tenha sido reposto depois.
		if storedShortfalls != nil {
			if err := json.Unmarshal([]byte(*storedShortfalls), &stored.Shortfalls); err != nil {
				return ReservationResult{}, fmt.Errorf("erro ao ler faltas gravadas: %w", err)
			}

			return stored, nil
		}
	}

	available, err := lockProducts(ctx, tx, requested)
	if err != nil {
		return ReservationResult{}, err
	}

	shortfalls := findShortfalls(requested, available)

	if alreadyProcessed {
		// Linhas anteriores à migration 014 não guardam as faltas.
		return ReservationResult{
			Status:     ReservationUnavailable,
			Shortfalls: shortfalls,
			Published:  published,
		}, nil
	}

	if len(shortfalls) > 0 {
		encodedShortfalls, err := json.Marshal(shortfalls)
		if err != nil {
			return ReservationResult{}, fmt.Errorf("erro ao serializar faltas: %w", err)
		}

		if _, err := tx.Exec(
			ctx,
			`INSERT INTO estoque.stock_reservations (order_id, status, shortfalls, request_payload)
			 VALUES ($1, $2, $3, $4::jsonb)`,
			orderID,
			ReservationUnavailable,
			string(encodedShortfalls),
			snapshot,
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
		`INSERT INTO estoque.stock_reservations (order_id, status, request_payload)
		 VALUES ($1, $2, $3::jsonb)
		 RETURNING created_at`,
		orderID,
		ReservationReserved,
		snapshot,
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

// SaveResultEnvelope stores the signed result event of an order. The first
// envelope saved wins, so every resend uses the same event_id; the stored one
// is returned.
func (repository *Repository) SaveResultEnvelope(
	ctx context.Context,
	orderID string,
	envelope string,
) (string, error) {
	var stored string

	err := repository.pool.QueryRow(
		ctx,
		// COALESCE mantém o envelope já gravado; um segundo gravador concorrente
		// espera o lock, relê a linha e recebe o envelope do primeiro.
		`UPDATE estoque.stock_reservations
		    SET result_envelope = COALESCE(result_envelope, $2),
		        updated_at = CASE WHEN result_envelope IS NULL THEN NOW() ELSE updated_at END
		  WHERE order_id = $1
		  RETURNING result_envelope`,
		orderID,
		envelope,
	).Scan(&stored)
	if err != nil {
		return "", fmt.Errorf("erro ao gravar resultado do pedido %s: %w", orderID, err)
	}

	return stored, nil
}

func (repository *Repository) MarkResultPublished(ctx context.Context, orderID string) error {
	if _, err := repository.pool.Exec(
		ctx,
		`UPDATE estoque.stock_reservations
		    SET result_published_at = COALESCE(result_published_at, NOW())
		  WHERE order_id = $1`,
		orderID,
	); err != nil {
		return fmt.Errorf("erro ao marcar publicação do pedido %s: %w", orderID, err)
	}

	return nil
}

// ListPendingResults returns stored result events not confirmed by the broker,
// older than minimumAge. Released orders are skipped: their result no longer
// matters to the Gateway.
func (repository *Repository) ListPendingResults(
	ctx context.Context,
	minimumAge time.Duration,
	limit int,
) ([]PendingResult, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT order_id, result_envelope
		   FROM estoque.stock_reservations
		  WHERE result_envelope IS NOT NULL
		    AND result_published_at IS NULL
		    AND status IN ('RESERVED', 'UNAVAILABLE')
		    AND updated_at <= NOW() - $1::interval
		  ORDER BY updated_at
		  LIMIT $2`,
		minimumAge,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar resultados pendentes: %w", err)
	}
	defer rows.Close()

	pending := make([]PendingResult, 0)

	for rows.Next() {
		var result PendingResult
		if err := rows.Scan(&result.OrderID, &result.Envelope); err != nil {
			return nil, fmt.Errorf("erro ao ler resultado pendente: %w", err)
		}
		pending = append(pending, result)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura dos resultados pendentes: %w", err)
	}

	return pending, nil
}
