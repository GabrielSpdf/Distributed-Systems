package orders

import (
	"context"
	"os"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExpireNextCheckoutIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("defina TEST_DATABASE_URL para executar a integração")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "ecommerce_events_test" {
		t.Fatal("use somente o banco isolado ecommerce_events_test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	// Protege pedidos que já existiam no banco de teste.
	// O método usa SKIP LOCKED e processará somente nossos novos registros.
	guard, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Rollback(ctx)

	if _, err := guard.Exec(ctx,
		"SELECT id FROM gateway.orders FOR UPDATE",
	); err != nil {
		t.Fatal(err)
	}

	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO gateway.users (name, email, password_hash)
		VALUES (
			'Teste de expiração',
			gen_random_uuid()::text || '@example.test',
			'conta-exclusiva-de-teste'
		)
		RETURNING id::text
	`).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, query := range []string{
			`DELETE FROM gateway.event_inbox WHERE customer_id = $1`,
			`DELETE FROM gateway.event_outbox
			 WHERE order_id IN (
			     SELECT id FROM gateway.orders WHERE user_id = $1
			 )`,
			`DELETE FROM gateway.orders WHERE user_id = $1`,
			`DELETE FROM gateway.users WHERE id = $1`,
		} {
			if _, err := pool.Exec(cleanupCtx, query, userID); err != nil {
				t.Errorf("erro na limpeza: %v", err)
			}
		}
	}()

	type fixture struct {
		name            string
		status          string
		expired         bool
		pendingPayment  bool
		databaseID      string
		displayID       string
		recentlyExpired bool
	}

	fixtures := []fixture{
		{name: "vencido", status: StatusAwaitingPayment, expired: true},
		{name: "prazo_valido", status: StatusAwaitingPayment},
		{name: "ja_aprovado", status: StatusPaymentApproved, expired: true},
		{
			name:           "pagamento_na_inbox",
			status:         StatusAwaitingPayment,
			expired:        true,
			pendingPayment: true,
		},
		{
			name:            "vencido_recentemente",
			status:          StatusAwaitingPayment,
			recentlyExpired: true,
		},
	}

	for i := range fixtures {
		f := &fixtures[i]
		expiresAt := time.Now().UTC().Add(time.Hour)
		if f.expired {
			expiresAt = time.Now().UTC().Add(-time.Minute)
		}

		if f.recentlyExpired {
			expiresAt = time.Now().UTC().Add(-5 * time.Second)
		}

		if err := pool.QueryRow(ctx, `
			INSERT INTO gateway.orders (
				display_id, user_id, status, total,
				checkout_url, charge_id, checkout_expires_at
			)
			VALUES (
				'EXP-' || nextval('gateway.order_display_id_seq')::text,
				$1, $2, 4.90,
				'http://localhost:8083/checkout/teste',
				gen_random_uuid()::text, $3
			)
			RETURNING id::text, display_id
		`, userID, f.status, expiresAt).Scan(
			&f.databaseID, &f.displayID,
		); err != nil {
			t.Fatal(err)
		}

		if f.pendingPayment {
			// Fixture de seleção: não será aplicada pelo processador de inbox.
			if _, err := pool.Exec(ctx, `
				INSERT INTO gateway.event_inbox (
					event_id, event_type, order_id,
					customer_id, envelope, status
				)
				VALUES (
					gen_random_uuid()::text, $1, $2,
					$3, '{}'::jsonb, 'RECEBIDO'
				)
			`, events.PaymentApproved, f.displayID, userID); err != nil {
				t.Fatal(err)
			}
		}
	}

	repository := NewRepository(pool)

	expired, err := repository.ExpireNextCheckout(ctx)
	if err != nil || !expired {
		t.Fatalf("expiração: alterou=%v, erro=%v", expired, err)
	}

	// Os outros pedidos não são elegíveis; não deve repetir o cancelamento.
	expired, err = repository.ExpireNextCheckout(ctx)
	if err != nil || expired {
		t.Fatalf("repetição: alterou=%v, erro=%v", expired, err)
	}

	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			var status string
			var outboxCount, historyCount int

			if err := pool.QueryRow(ctx, `
				SELECT o.status,
				       (SELECT COUNT(*) FROM gateway.event_outbox
				        WHERE order_id = o.id),
				       (SELECT COUNT(*) FROM gateway.order_status_history
				        WHERE order_id = o.id)
				FROM gateway.orders o
				WHERE o.id = $1
			`, f.databaseID).Scan(
				&status, &outboxCount, &historyCount,
			); err != nil {
				t.Fatal(err)
			}

			expectedStatus := f.status
			expectedCount := 0
			if f.name == "vencido" {
				expectedStatus = StatusCancelled
				expectedCount = 1
			}

			if status != expectedStatus ||
				outboxCount != expectedCount ||
				historyCount != expectedCount {
				t.Fatalf(
					"status=%s, outbox=%d, histórico=%d; esperados=%s/%d/%d",
					status, outboxCount, historyCount,
					expectedStatus, expectedCount, expectedCount,
				)
			}
		})
	}

	var eventType, reason, orderID, customerID, outboxStatus string
	var attempts int

	if err := pool.QueryRow(ctx, `
		SELECT event_type, payload->>'reason', payload->>'order_id',
		       payload->>'customer_id', status, attempt_count
		FROM gateway.event_outbox
		WHERE order_id = $1
	`, fixtures[0].databaseID).Scan(
		&eventType, &reason, &orderID,
		&customerID, &outboxStatus, &attempts,
	); err != nil {
		t.Fatal(err)
	}

	if eventType != events.OrderDeleted ||
		reason != events.ReasonCheckoutExpired ||
		orderID != fixtures[0].displayID ||
		customerID != userID ||
		outboxStatus != "PENDENTE" ||
		attempts != 0 {
		t.Fatal("compensação de expiração gravada incorretamente")
	}
}
