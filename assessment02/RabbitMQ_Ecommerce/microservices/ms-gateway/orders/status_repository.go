package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidStatusEvent = errors.New("evento de status inválido")
	ErrStatusTransition   = errors.New("transição de status incompatível")
)

// ApplyStatusEvent retorna true quando uma atualização foi confirmada.
// Evento já processado retorna false, nil.
// A assinatura deve ser verificada pelo consumidor antes desta chamada.
func (repository *Repository) applyStatusEventTx(
	ctx context.Context,
	tx pgx.Tx,
	eventID string,
	eventType string,
	data json.RawMessage,
) (bool, error) {
	if strings.TrimSpace(eventID) == "" || len(eventID) > 80 {
		return false, fmt.Errorf(
			"%w: identificador do evento inválido",
			ErrInvalidStatusEvent,
		)
	}

	event, err := ReadStatusEvent(eventType, data)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidStatusEvent, err)
	}

	var databaseID, userID, currentStatus, currentChargeID string
	var totalCents int64

	err = tx.QueryRow(ctx, `
		SELECT id, user_id::text, status,
		       (total * 100)::bigint, COALESCE(charge_id, '')
		FROM gateway.orders
		WHERE display_id = $1 AND is_deleted = FALSE
		FOR UPDATE
	`, event.OrderID).Scan(
		&databaseID,
		&userID,
		&currentStatus,
		&totalCents,
		&currentChargeID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrOrderNotFound
	}
	if err != nil {
		return false, fmt.Errorf("erro ao bloquear pedido: %w", err)
	}

	if userID != event.CustomerID {
		return false, fmt.Errorf(
			"%w: cliente não corresponde ao pedido",
			ErrInvalidStatusEvent,
		)
	}

	var alreadyProcessed bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM gateway.order_status_history
			WHERE order_id = $1 AND event_id = $2
		)
	`, databaseID, eventID).Scan(&alreadyProcessed)
	if err != nil {
		return false, fmt.Errorf("erro ao verificar duplicação: %w", err)
	}
	if alreadyProcessed {
		return false, nil
	}

	nextStatus, err := NextOrderStatus(currentStatus, eventType)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrStatusTransition, err)
	}

	if event.StockConfirmed != nil &&
		!matchesTotalCents(event.StockConfirmed.Total, totalCents) {
		return false, fmt.Errorf(
			"%w: total do estoque difere do pedido",
			ErrInvalidStatusEvent,
		)
	}

	if event.Payment != nil {
		if !matchesTotalCents(event.Payment.Amount, totalCents) {
			return false, fmt.Errorf(
				"%w: valor do pagamento difere do pedido",
				ErrInvalidStatusEvent,
			)
		}
		if currentChargeID == "" ||
			currentChargeID != event.Payment.ChargeID {
			return false, fmt.Errorf(
				"%w: cobrança não corresponde ao pedido",
				ErrInvalidStatusEvent,
			)
		}
	}

	if event.StockUnavailable != nil {
		for _, item := range event.StockUnavailable.UnavailableItems {
			var matches bool
			err = tx.QueryRow(ctx, `
				SELECT EXISTS (
					SELECT 1
					FROM gateway.order_items
					WHERE order_id = $1
					  AND product_id = $2
					  AND quantity = $3
				)
			`, databaseID, item.ProductID, item.RequestedQuantity).Scan(&matches)
			if err != nil {
				return false, fmt.Errorf("erro ao conferir item: %w", err)
			}
			if !matches {
				return false, fmt.Errorf(
					"%w: item indisponível não corresponde ao pedido",
					ErrInvalidStatusEvent,
				)
			}
		}
	}

	var checkoutURL, chargeID, invoiceID, trackingCode *string
	var checkoutExpiresAt, shippedAt *time.Time

	if event.Checkout != nil {
		checkoutURL = &event.Checkout.CheckoutURL
		chargeID = &event.Checkout.ChargeID
		checkoutExpiresAt = &event.Checkout.ExpiresAt
	}

	if event.Shipped != nil {
		invoiceID = &event.Shipped.InvoiceID
		trackingCode = &event.Shipped.TrackingCode
		shippedAt = &event.Shipped.ShippedAt
	}

	var updatedAt time.Time
	err = tx.QueryRow(ctx, `
		UPDATE gateway.orders
		SET status = $2,
		    checkout_url = COALESCE($3::text, checkout_url),
		    charge_id = COALESCE($4::text, charge_id),
		    checkout_expires_at =
		        COALESCE($5::timestamptz, checkout_expires_at),
		    invoice_id = COALESCE($6::text, invoice_id),
		    tracking_code = COALESCE($7::text, tracking_code),
		    shipped_at = COALESCE($8::timestamptz, shipped_at),
		    updated_at = clock_timestamp()
		WHERE id = $1
		RETURNING updated_at
	`,
		databaseID,
		nextStatus,
		checkoutURL,
		chargeID,
		checkoutExpiresAt,
		invoiceID,
		trackingCode,
		shippedAt,
	).Scan(&updatedAt)
	if err != nil {
		return false, fmt.Errorf("erro ao atualizar pedido: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO gateway.order_status_history (
			order_id, status, event_id, created_at
		)
		VALUES ($1, $2, $3, $4)
	`, databaseID, nextStatus, eventID, updatedAt)
	if err != nil {
		return false, fmt.Errorf("erro ao registrar histórico: %w", err)
	}

	if event.EventType == events.PaymentRefused {
		err := repository.enqueueOrderDeletedTx(
			ctx,
			tx,
			databaseID,
			events.WebOrderDeletedPayload{
				OrderID:     event.OrderID,
				CustomerID:  userID,
				Reason:      events.ReasonPaymentRefused,
				CancelledAt: updatedAt.UTC(),
			},
		)
		if err != nil {
			return false, err
		}
	}

	return true, nil
}

func matchesTotalCents(amount float64, expectedCents int64) bool {
	if amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return false
	}

	scaled := amount * 100
	return math.Abs(scaled-float64(expectedCents)) < 0.000001
}

// StoreStatusEvent guarda um envelope após verificação de assinatura
// e produtor autorizado pelo consumidor.
// Retorna true para evento novo e false para repetição idêntica.
func (repository *Repository) StoreStatusEvent(
	ctx context.Context,
	envelope events.EventEnvelope,
) (bool, error) {
	if strings.TrimSpace(envelope.EventID) == "" ||
		len(envelope.EventID) > 80 {
		return false, fmt.Errorf(
			"%w: identificador do evento inválido",
			ErrInvalidStatusEvent,
		)
	}

	if envelope.Timestamp.IsZero() ||
		strings.TrimSpace(envelope.Producer) == "" ||
		strings.TrimSpace(envelope.Signature) == "" {
		return false, fmt.Errorf(
			"%w: envelope incompleto",
			ErrInvalidStatusEvent,
		)
	}

	event, err := ReadStatusEvent(envelope.EventType, envelope.Payload)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidStatusEvent, err)
	}

	if len(event.OrderID) > 32 {
		return false, fmt.Errorf(
			"%w: identificador do pedido excede o limite",
			ErrInvalidStatusEvent,
		)
	}

	var customerID pgtype.UUID
	if err := customerID.Scan(event.CustomerID); err != nil ||
		!customerID.Valid {
		return false, fmt.Errorf(
			"%w: cliente deve ter identificador UUID válido",
			ErrInvalidStatusEvent,
		)
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return false, fmt.Errorf(
			"%w: erro ao serializar envelope: %v",
			ErrInvalidStatusEvent,
			err,
		)
	}

	result, err := repository.pool.Exec(ctx, `
		INSERT INTO gateway.event_inbox (
			event_id, event_type, order_id, customer_id, envelope
		)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (event_id) DO NOTHING
	`,
		envelope.EventID,
		envelope.EventType,
		event.OrderID,
		customerID,
		string(data),
	)
	if err != nil {
		return false, fmt.Errorf("erro ao guardar evento: %w", err)
	}

	if result.RowsAffected() == 1 {
		return true, nil
	}

	var identical bool
	err = repository.pool.QueryRow(ctx, `
		SELECT envelope = $2::jsonb
		FROM gateway.event_inbox
		WHERE event_id = $1
	`, envelope.EventID, string(data)).Scan(&identical)
	if err != nil {
		return false, fmt.Errorf("erro ao conferir evento repetido: %w", err)
	}

	if !identical {
		return false, fmt.Errorf(
			"%w: identificador reutilizado com envelope diferente",
			ErrInvalidStatusEvent,
		)
	}

	return false, nil
}

// ApplyStatusEvent inicia uma transação para aplicar um evento.
// Retorna false, nil quando o evento já foi processado.
func (repository *Repository) ApplyStatusEvent(
	ctx context.Context,
	eventID string,
	eventType string,
	data json.RawMessage,
) (bool, error) {
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar transação: %w", err)
	}
	defer tx.Rollback(ctx)

	applied, err := repository.applyStatusEventTx(
		ctx, tx, eventID, eventType, data,
	)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao confirmar atualização: %w", err)
	}

	if applied {
		repository.notifyAppliedStatusEvent(eventID, eventType, data)
	}

	return applied, nil
}

// ProcessNextStatusEvent retorna true quando tratou um evento da inbox.
// Retorna false, nil quando não há evento disponível.
func (repository *Repository) ProcessNextStatusEvent(
	ctx context.Context,
) (bool, error) {
	const maxAttempts = 12

	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("erro ao iniciar processamento: %w", err)
	}
	defer tx.Rollback(ctx)

	var eventID string
	var envelopeData []byte
	var attempts int

	err = tx.QueryRow(ctx, `
		SELECT event_id, envelope, attempt_count
		FROM gateway.event_inbox
		WHERE status = 'RECEBIDO'
		  AND next_attempt_at <= clock_timestamp()
		ORDER BY next_attempt_at, received_at, event_id
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`).Scan(&eventID, &envelopeData, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("erro ao buscar evento pendente: %w", err)
	}

	var envelope events.EventEnvelope
	var applied bool
	processErr := json.Unmarshal(envelopeData, &envelope)
	if processErr != nil {
		processErr = fmt.Errorf(
			"%w: envelope armazenado inválido: %v",
			ErrInvalidStatusEvent,
			processErr,
		)
	} else if envelope.EventID != eventID {
		processErr = fmt.Errorf(
			"%w: identificador armazenado inconsistente",
			ErrInvalidStatusEvent,
		)
	} else {
		// Transação aninhada do pgx cria um savepoint.
		attemptTx, err := tx.Begin(ctx)
		if err != nil {
			return false, fmt.Errorf("erro ao criar savepoint: %w", err)
		}

		applied, processErr = repository.applyStatusEventTx(
			ctx,
			attemptTx,
			envelope.EventID,
			envelope.EventType,
			envelope.Payload,
		)

		if processErr != nil {
			if err := attemptTx.Rollback(ctx); err != nil {
				return false, fmt.Errorf(
					"erro ao desfazer tentativa: %w", err,
				)
			}
		} else {
			if err := attemptTx.Commit(ctx); err != nil {
				return false, fmt.Errorf(
					"erro ao concluir tentativa: %w", err,
				)
			}
		}
	}

	nextStatus := "PROCESSADO"
	var lastError *string

	if processErr != nil {
		message := processErr.Error()
		lastError = &message

		switch {
		case errors.Is(processErr, ErrInvalidStatusEvent):
			nextStatus = "REJEITADO"

		case errors.Is(processErr, ErrStatusTransition),
			errors.Is(processErr, ErrOrderNotFound):
			nextStatus = "RECEBIDO"
			if attempts+1 >= maxAttempts {
				nextStatus = "PENDENTE_REVISAO"
			}

		default:
			// Falha operacional: não confirma nenhuma alteração.
			return false, fmt.Errorf(
				"erro ao processar evento %s: %w",
				eventID,
				processErr,
			)
		}
	}

	_, err = tx.Exec(ctx, `
		UPDATE gateway.event_inbox
		SET status = $2::text,
		    attempt_count = attempt_count + 1,
		    last_error = $3::text,
		    next_attempt_at =
		        clock_timestamp() + INTERVAL '10 seconds',
		    processed_at = CASE
		        WHEN $2::text = 'PROCESSADO' THEN clock_timestamp()
		        ELSE NULL
		    END
		WHERE event_id = $1
	`, eventID, nextStatus, lastError)
	if err != nil {
		return false, fmt.Errorf("erro ao atualizar inbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("erro ao confirmar processamento: %w", err)
	}

	if applied && processErr == nil {
		repository.notifyAppliedStatusEvent(
			envelope.EventID,
			envelope.EventType,
			envelope.Payload,
		)
	}

	return true, nil
}
