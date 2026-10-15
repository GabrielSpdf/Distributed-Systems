package mspagamento

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/database"

	"github.com/jackc/pgx/v5/pgxpool"
)

func openTestRepository(t *testing.T) (*Repository, *pgxpool.Pool, string) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL não definida; teste de integração ignorado")
	}

	pool, err := database.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("erro ao conectar ao banco de teste: %v", err)
	}
	t.Cleanup(pool.Close)

	prefix := "TEST-PAG-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-"

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = pool.Exec(ctx, `
			DELETE FROM pagamento.webhook_events
			 WHERE charge_id IN (
			       SELECT id FROM pagamento.payment_charges WHERE order_id LIKE $1)`,
			prefix+"%",
		)
		_, _ = pool.Exec(ctx,
			"DELETE FROM pagamento.payment_charges WHERE order_id LIKE $1",
			prefix+"%",
		)
	})

	return NewRepository(pool), pool, prefix
}

func newChargeID(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var id string
	if err := pool.QueryRow(context.Background(), "SELECT gen_random_uuid()::text").Scan(&id); err != nil {
		t.Fatalf("erro ao gerar uuid: %v", err)
	}

	return id
}

func pendingCharge(t *testing.T, pool *pgxpool.Pool, orderID string) Charge {
	return Charge{
		ID:          newChargeID(t, pool),
		OrderID:     orderID,
		CustomerID:  "083a7808-462b-4df3-a755-3130bc571351",
		Amount:      59.80,
		Currency:    "BRL",
		Status:      ChargePending,
		CheckoutURL: "http://localhost:8083/checkout/x",
		ExpiresAt:   time.Date(2026, 10, 1, 18, 31, 0, 0, time.UTC),
	}
}

func saved(t *testing.T, repository *Repository, charge Charge) Charge {
	t.Helper()

	stored, err := repository.SaveCharge(context.Background(), charge)
	if err != nil {
		t.Fatalf("erro ao salvar cobrança: %v", err)
	}

	return stored
}

func TestSaveAndFindCharge(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()

	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	found, exists, err := repository.FindChargeByOrder(ctx, prefix+"1")
	if err != nil || !exists {
		t.Fatalf("cobrança não encontrada: %v", err)
	}

	if found.ID != charge.ID || found.Amount != 59.80 || found.CustomerID != charge.CustomerID ||
		found.Status != ChargePending || !found.ExpiresAt.Equal(charge.ExpiresAt) ||
		found.CheckoutPublished || found.ResultPublished {
		t.Fatalf("cobrança inesperada: %+v", found)
	}

	again := saved(t, repository, pendingCharge(t, pool, prefix+"1"))
	if again.ID != charge.ID {
		t.Fatalf("segunda cobrança para o mesmo pedido: %s != %s", again.ID, charge.ID)
	}

	_, exists, err = repository.FindChargeByOrder(ctx, prefix+"inexistente")
	if err != nil || exists {
		t.Fatalf("esperava ausência, recebeu exists=%v err=%v", exists, err)
	}
}

func TestMarkPublishedFlags(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	if err := repository.MarkCheckoutPublished(ctx, charge.ID); err != nil {
		t.Fatalf("erro ao marcar checkout: %v", err)
	}

	if err := repository.MarkResultPublished(ctx, charge.ID); err != nil {
		t.Fatalf("erro ao marcar resultado: %v", err)
	}

	found, _, _ := repository.FindChargeByOrder(ctx, prefix+"1")
	if !found.CheckoutPublished || !found.ResultPublished {
		t.Fatalf("marcações não gravadas: %+v", found)
	}
}

func webhookFor(charge Charge, eventID string, status string) WebhookEvent {
	return WebhookEvent{
		EventID:    eventID,
		ChargeID:   charge.ID,
		OrderID:    charge.OrderID,
		Status:     status,
		OccurredAt: time.Date(2026, 10, 1, 18, 10, 0, 0, time.UTC),
	}
}

func TestApplyWebhookStoresResultOnce(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	outcome, err := repository.ApplyWebhook(ctx, webhookFor(charge, prefix+"whk-1", ChargeRefused), "Pagamento recusado pelo usuário")
	if err != nil {
		t.Fatalf("erro ao aplicar webhook: %v", err)
	}

	if outcome.Duplicate || outcome.Charge.Status != ChargeRefused ||
		outcome.Charge.Reason != "Pagamento recusado pelo usuário" ||
		!outcome.Charge.ProcessedAt.Equal(time.Date(2026, 10, 1, 18, 10, 0, 0, time.UTC)) {
		t.Fatalf("resultado inesperado: %+v", outcome)
	}

	repeated, err := repository.ApplyWebhook(ctx, webhookFor(charge, prefix+"whk-1", ChargeRefused), "Pagamento recusado pelo usuário")
	if err != nil || !repeated.Duplicate || repeated.Charge.Status != ChargeRefused {
		t.Fatalf("webhook repetido inesperado: %+v %v", repeated, err)
	}

	sameResult, err := repository.ApplyWebhook(ctx, webhookFor(charge, prefix+"whk-2", ChargeRefused), "Pagamento recusado pelo usuário")
	if err != nil || !sameResult.Duplicate {
		t.Fatalf("mesmo resultado com outro evento inesperado: %+v %v", sameResult, err)
	}
}

func TestApplyWebhookRejectsConflictingResult(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	if _, err := repository.ApplyWebhook(ctx, webhookFor(charge, prefix+"whk-1", ChargeApproved), ""); err != nil {
		t.Fatalf("erro ao aprovar: %v", err)
	}

	_, err := repository.ApplyWebhook(ctx, webhookFor(charge, prefix+"whk-2", ChargeRefused), "recusado")
	if !errors.Is(err, ErrConflictingResult) {
		t.Fatalf("esperava ErrConflictingResult, recebeu %v", err)
	}

	found, _, _ := repository.FindChargeByOrder(ctx, prefix+"1")
	if found.Status != ChargeApproved {
		t.Fatalf("resultado conflitante alterou a cobrança: %s", found.Status)
	}
}

func TestApplyWebhookRejectsUnknownChargeAndOtherOrder(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	unknown := webhookFor(charge, prefix+"whk-1", ChargeApproved)
	unknown.ChargeID = newChargeID(t, pool)

	if _, err := repository.ApplyWebhook(ctx, unknown, ""); !errors.Is(err, ErrChargeNotFound) {
		t.Fatalf("esperava ErrChargeNotFound, recebeu %v", err)
	}

	otherOrder := webhookFor(charge, prefix+"whk-2", ChargeApproved)
	otherOrder.OrderID = prefix + "outro"

	if _, err := repository.ApplyWebhook(ctx, otherOrder, ""); !errors.Is(err, ErrInvalidWebhook) {
		t.Fatalf("esperava ErrInvalidWebhook, recebeu %v", err)
	}

	found, _, _ := repository.FindChargeByOrder(ctx, prefix+"1")
	if found.Status != ChargePending {
		t.Fatalf("webhook rejeitado alterou a cobrança: %s", found.Status)
	}
}
