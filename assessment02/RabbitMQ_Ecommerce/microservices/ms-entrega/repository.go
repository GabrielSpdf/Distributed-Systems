package msentrega

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository owns every query against the entrega schema.
type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{
		pool: pool,
	}
}

const shipmentColumns = `
	order_id,
	customer_id,
	charge_id,
	invoice_id,
	tracking_code,
	shipped_at,
	envelope,
	published_at IS NOT NULL
`

func scanShipment(row pgx.Row) (Shipment, error) {
	var shipment Shipment

	err := row.Scan(
		&shipment.OrderID,
		&shipment.CustomerID,
		&shipment.ChargeID,
		&shipment.InvoiceID,
		&shipment.TrackingCode,
		&shipment.ShippedAt,
		&shipment.Envelope,
		&shipment.Published,
	)
	if err != nil {
		return Shipment{}, err
	}

	shipment.ShippedAt = shipment.ShippedAt.UTC()

	return shipment, nil
}

func (repository *Repository) FindShipment(
	ctx context.Context,
	orderID string,
) (Shipment, bool, error) {
	shipment, err := scanShipment(repository.pool.QueryRow(
		ctx,
		`SELECT `+shipmentColumns+`
		   FROM entrega.shipments
		  WHERE order_id = $1`,
		orderID,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return Shipment{}, false, nil
	}

	if err != nil {
		return Shipment{}, false, fmt.Errorf("erro ao consultar entrega: %w", err)
	}

	return shipment, true, nil
}

// CreateShipment stores a new shipment. If the order already has one, the
// stored shipment is returned unchanged, so an order never ships twice.
func (repository *Repository) CreateShipment(
	ctx context.Context,
	shipment Shipment,
) (Shipment, error) {
	if _, err := repository.pool.Exec(
		ctx,
		`INSERT INTO entrega.shipments
		     (order_id, customer_id, charge_id, invoice_id, tracking_code, shipped_at, envelope)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (order_id) DO NOTHING`,
		shipment.OrderID,
		shipment.CustomerID,
		shipment.ChargeID,
		shipment.InvoiceID,
		shipment.TrackingCode,
		shipment.ShippedAt,
		shipment.Envelope,
	); err != nil {
		return Shipment{}, fmt.Errorf("erro ao gravar entrega: %w", err)
	}

	stored, exists, err := repository.FindShipment(ctx, shipment.OrderID)
	if err != nil {
		return Shipment{}, err
	}

	if !exists {
		return Shipment{}, fmt.Errorf("entrega do pedido %s não encontrada após gravação", shipment.OrderID)
	}

	return stored, nil
}

// ListPendingShipments returns shipments whose event was not confirmed by the
// broker and that are older than minimumAge, so the recovery routine does not
// race with the consumer that has just created them.
func (repository *Repository) ListPendingShipments(
	ctx context.Context,
	minimumAge time.Duration,
	limit int,
) ([]Shipment, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT `+shipmentColumns+`
		   FROM entrega.shipments
		  WHERE published_at IS NULL
		    AND created_at <= NOW() - $1::interval
		  ORDER BY created_at
		  LIMIT $2`,
		minimumAge,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar entregas pendentes: %w", err)
	}
	defer rows.Close()

	pending := make([]Shipment, 0)

	for rows.Next() {
		shipment, err := scanShipment(rows)
		if err != nil {
			return nil, fmt.Errorf("erro ao ler entrega pendente: %w", err)
		}

		pending = append(pending, shipment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante leitura das pendências: %w", err)
	}

	return pending, nil
}

func (repository *Repository) MarkPublished(ctx context.Context, orderID string) error {
	if _, err := repository.pool.Exec(
		ctx,
		`UPDATE entrega.shipments
		    SET published_at = COALESCE(published_at, NOW())
		  WHERE order_id = $1`,
		orderID,
	); err != nil {
		return fmt.Errorf("erro ao marcar publicação da entrega %s: %w", orderID, err)
	}

	return nil
}
