package orders

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

// authorityFixture prepara um pedido em AGUARDANDO_PAGAMENTO no banco
// ecommerce_events_test, removendo só os registros do usuário criado.
type authorityFixture struct {
	ctx        context.Context
	pool       *pgxpool.Pool
	repository *Repository
	userID     string
	order      Order
}

func newAuthorityFixture(t *testing.T) *authorityFixture {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("defina TEST_DATABASE_URL para executar o teste de integração")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	var databaseName string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "ecommerce_events_test" {
		t.Fatal("execute este teste somente no banco ecommerce_events_test")
	}

	fixture := &authorityFixture{ctx: ctx, pool: pool, repository: NewRepository(pool)}

	err = pool.QueryRow(ctx, `
		INSERT INTO gateway.users (name, email, password_hash)
		VALUES ('Teste de autoridade', gen_random_uuid()::text || '@example.test', 'conta-exclusiva-de-teste')
		RETURNING id
	`).Scan(&fixture.userID)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()

		pool.Exec(cleanupCtx, "DELETE FROM gateway.event_inbox WHERE customer_id = $1", fixture.userID)
		pool.Exec(cleanupCtx, `
			DELETE FROM gateway.event_outbox
			WHERE order_id IN (SELECT id FROM gateway.orders WHERE user_id = $1)
		`, fixture.userID)
		pool.Exec(cleanupCtx, "DELETE FROM gateway.orders WHERE user_id = $1", fixture.userID)
		pool.Exec(cleanupCtx, "DELETE FROM gateway.users WHERE id = $1", fixture.userID)
	})

	fixture.order, err = fixture.repository.Create(ctx, fixture.userID, []OrderItem{
		{ProductID: "PROD-002", ProductName: "Detergente", Quantity: 1, UnitPrice: 4.9},
	}, 490)
	if err != nil {
		t.Fatal(err)
	}

	fixture.apply(t, "autoridade-estoque", events.OrderStockConfirmed, events.WebStockConfirmedPayload{
		OrderID:    fixture.order.ID,
		CustomerID: fixture.userID,
		Total:      4.9,
		Currency:   "BRL",
		ReservedAt: time.Now().UTC(),
	})
	fixture.apply(t, "autoridade-checkout", events.PaymentCheckoutAvailable, events.WebPaymentCheckoutPayload{
		OrderID:     fixture.order.ID,
		CustomerID:  fixture.userID,
		ChargeID:    "COB-AUTORIDADE",
		CheckoutURL: "https://checkout.example.com/COB-AUTORIDADE",
		ExpiresAt:   time.Now().UTC().Add(time.Hour),
	})

	return fixture
}

func (fixture *authorityFixture) apply(t *testing.T, eventID string, eventType string, value any) {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}

	applied, err := fixture.repository.ApplyStatusEvent(fixture.ctx, eventID, eventType, data)
	if err != nil || !applied {
		t.Fatalf("%s: aplicado=%v, erro=%v", eventType, applied, err)
	}
}

func (fixture *authorityFixture) payment(reason string) events.WebPaymentResultPayload {
	return events.WebPaymentResultPayload{
		OrderID:     fixture.order.ID,
		CustomerID:  fixture.userID,
		ChargeID:    "COB-AUTORIDADE",
		Amount:      4.9,
		Currency:    "BRL",
		Reason:      reason,
		ProcessedAt: time.Now().UTC(),
	}
}

func TestCheckoutExpiredRefusalCancelsOrder(t *testing.T) {
	fixture := newAuthorityFixture(t)

	statusSSE, unsubscribe := fixture.repository.sseHub.subscribe(fixture.userID)
	defer unsubscribe()

	fixture.apply(t, "autoridade-expirado", events.PaymentRefused, fixture.payment(events.ReasonCheckoutExpired))

	var status string
	if err := fixture.pool.QueryRow(fixture.ctx,
		"SELECT status FROM gateway.orders WHERE id = $1", fixture.order.DatabaseID,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusCancelled {
		t.Fatalf("checkout expirado deveria cancelar o pedido; estado=%s", status)
	}

	var payloadData []byte
	if err := fixture.pool.QueryRow(fixture.ctx,
		"SELECT payload FROM gateway.event_outbox WHERE order_id = $1", fixture.order.DatabaseID,
	).Scan(&payloadData); err != nil {
		t.Fatal(err)
	}

	var compensation events.WebOrderDeletedPayload
	if err := json.Unmarshal(payloadData, &compensation); err != nil {
		t.Fatal(err)
	}
	if compensation.Reason != events.ReasonCheckoutExpired {
		t.Fatalf("compensação com motivo inesperado: %s", compensation.Reason)
	}

	select {
	case event := <-statusSSE:
		var notified struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(event.Data, &notified); err != nil || notified.Status != StatusCancelled {
			t.Fatalf("aviso SSE inesperado: %s %v", event.Data, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nenhum aviso SSE para o cancelamento")
	}
}

func TestOrdinaryRefusalKeepsPaymentRefused(t *testing.T) {
	fixture := newAuthorityFixture(t)

	fixture.apply(t, "autoridade-recusa", events.PaymentRefused, fixture.payment("Pagamento recusado pelo usuário"))

	var status string
	if err := fixture.pool.QueryRow(fixture.ctx,
		"SELECT status FROM gateway.orders WHERE id = $1", fixture.order.DatabaseID,
	).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusPaymentRefused {
		t.Fatalf("recusa comum deveria continuar PAGAMENTO_RECUSADO; estado=%s", status)
	}
}

func TestOrderDetailsIncludeTrackingAfterShipping(t *testing.T) {
	fixture := newAuthorityFixture(t)
	shippedAt := time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC)

	fixture.apply(t, "autoridade-aprovado", events.PaymentApproved, fixture.payment(""))
	fixture.apply(t, "autoridade-enviado", events.OrderShipped, events.WebOrderShippedPayload{
		OrderID:      fixture.order.ID,
		CustomerID:   fixture.userID,
		InvoiceID:    "NF-20261015-123456",
		TrackingCode: "BR123456789BR",
		ShippedAt:    shippedAt,
	})

	details, err := fixture.repository.FindByID(fixture.ctx, fixture.userID, fixture.order.ID)
	if err != nil {
		t.Fatal(err)
	}

	if details.InvoiceID != "NF-20261015-123456" || details.TrackingCode != "BR123456789BR" ||
		details.ShippedAt == nil || !details.ShippedAt.Equal(shippedAt) {
		t.Fatalf("detalhes sem os dados de envio: %+v", details)
	}

	list, err := fixture.repository.ListByUser(fixture.ctx, fixture.userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].TrackingCode != "BR123456789BR" {
		t.Fatalf("listagem sem o rastreio: %+v", list)
	}

	encoded, _ := json.Marshal(details)
	var fields map[string]any
	_ = json.Unmarshal(encoded, &fields)
	if fields["invoice_id"] != "NF-20261015-123456" || fields["tracking_code"] != "BR123456789BR" ||
		fields["shipped_at"] == nil {
		t.Fatalf("JSON do pedido sem os campos de envio: %s", encoded)
	}
}
