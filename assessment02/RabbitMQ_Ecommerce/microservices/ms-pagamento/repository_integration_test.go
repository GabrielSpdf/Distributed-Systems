package mspagamento

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/events"

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

func TestApplyWebhookRejectsEventIDReusedForAnotherCharge(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	first := saved(t, repository, pendingCharge(t, pool, prefix+"1"))
	second := saved(t, repository, pendingCharge(t, pool, prefix+"2"))

	if _, err := repository.ApplyWebhook(ctx, webhookFor(first, prefix+"whk-1", ChargeApproved), ""); err != nil {
		t.Fatalf("erro ao aprovar a primeira cobrança: %v", err)
	}

	_, err := repository.ApplyWebhook(ctx, webhookFor(second, prefix+"whk-1", ChargeApproved), "")
	if !errors.Is(err, ErrWebhookEventReused) {
		t.Fatalf("esperava ErrWebhookEventReused, recebeu %v", err)
	}

	found, _, _ := repository.FindChargeByOrder(ctx, prefix+"2")
	if found.Status != ChargePending {
		t.Fatalf("evento reutilizado alterou outra cobrança: %s", found.Status)
	}
}

func TestEnvelopesFirstWriterWins(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	for _, save := range []func(context.Context, string, string) (string, error){
		repository.SaveCheckoutEnvelope,
		repository.SaveResultEnvelope,
	} {
		first, err := save(ctx, charge.ID, `{"event_id":"primeiro"}`)
		if err != nil || first != `{"event_id":"primeiro"}` {
			t.Fatalf("primeiro envelope não gravado: %q %v", first, err)
		}

		second, err := save(ctx, charge.ID, `{"event_id":"segundo"}`)
		if err != nil || second != `{"event_id":"primeiro"}` {
			t.Fatalf("segundo envelope substituiu o primeiro: %q %v", second, err)
		}
	}

	found, _, _ := repository.FindChargeByOrder(ctx, prefix+"1")
	if found.CheckoutEnvelope != `{"event_id":"primeiro"}` || found.ResultEnvelope != `{"event_id":"primeiro"}` {
		t.Fatalf("envelopes não aparecem na cobrança: %+v", found)
	}
}

func TestPendingCheckoutsAndResults(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	contains := func(charges []Charge, chargeID string) bool {
		for _, candidate := range charges {
			if candidate.ID == chargeID {
				return true
			}
		}
		return false
	}

	checkouts, err := repository.ListPendingCheckouts(ctx, 0, 1000)
	if err != nil || !contains(checkouts, charge.ID) {
		t.Fatalf("checkout não publicado deveria estar pendente: %v", err)
	}

	results, _ := repository.ListPendingResults(ctx, 0, 1000)
	if contains(results, charge.ID) {
		t.Fatal("cobrança sem resultado não tem resultado pendente")
	}

	if err := repository.MarkCheckoutPublished(ctx, charge.ID); err != nil {
		t.Fatalf("erro ao marcar checkout: %v", err)
	}

	if _, err := repository.ApplyWebhook(ctx, webhookFor(charge, prefix+"whk-1", ChargeApproved), ""); err != nil {
		t.Fatalf("erro ao aprovar: %v", err)
	}

	checkouts, _ = repository.ListPendingCheckouts(ctx, 0, 1000)
	results, err = repository.ListPendingResults(ctx, 0, 1000)
	if err != nil || contains(checkouts, charge.ID) || !contains(results, charge.ID) {
		t.Fatalf("esperava só o resultado pendente: checkout=%v resultado=%v err=%v",
			contains(checkouts, charge.ID), contains(results, charge.ID), err)
	}

	if err := repository.MarkResultPublished(ctx, charge.ID); err != nil {
		t.Fatalf("erro ao marcar resultado: %v", err)
	}

	results, _ = repository.ListPendingResults(ctx, 0, 1000)
	if contains(results, charge.ID) {
		t.Fatal("resultado publicado continua pendente")
	}
}

func TestExpireNextChargeOnlyExpiresOverduePending(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()

	overdue := saved(t, repository, pendingCharge(t, pool, prefix+"vencida"))

	future := pendingCharge(t, pool, prefix+"futura")
	future.ExpiresAt = time.Now().UTC().Add(time.Hour)
	future = saved(t, repository, future)

	approved := saved(t, repository, pendingCharge(t, pool, prefix+"aprovada"))
	if _, err := repository.ApplyWebhook(ctx, webhookFor(approved, prefix+"whk-1", ChargeApproved), ""); err != nil {
		t.Fatalf("erro ao aprovar: %v", err)
	}

	expired := map[string]Charge{}
	for {
		charge, found, err := repository.ExpireNextCharge(ctx, 0)
		if err != nil {
			t.Fatalf("erro ao expirar: %v", err)
		}
		if !found {
			break
		}
		expired[charge.ID] = charge
	}

	got, ok := expired[overdue.ID]
	if !ok || got.Status != ChargeExpired || got.Reason != events.ReasonCheckoutExpired || got.ProcessedAt.IsZero() {
		t.Fatalf("cobrança vencida não expirou corretamente: %+v", got)
	}

	if _, wrong := expired[future.ID]; wrong {
		t.Fatal("cobrança no prazo foi expirada")
	}

	if _, wrong := expired[approved.ID]; wrong {
		t.Fatal("cobrança aprovada foi expirada")
	}
}

func TestWebhookAfterExpiryIsRejected(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()

	late := saved(t, repository, pendingCharge(t, pool, prefix+"atrasado"))
	lateEvent := webhookFor(late, prefix+"whk-atrasado", ChargeApproved)
	lateEvent.OccurredAt = late.ExpiresAt.Add(time.Second)

	if _, err := repository.ApplyWebhook(ctx, lateEvent, ""); !errors.Is(err, ErrCheckoutExpired) {
		t.Fatalf("esperava ErrCheckoutExpired para decisão após o prazo, recebeu %v", err)
	}

	expired := saved(t, repository, pendingCharge(t, pool, prefix+"expirada"))
	if _, err := pool.Exec(ctx, "UPDATE pagamento.payment_charges SET status = 'EXPIRED' WHERE id = $1::uuid", expired.ID); err != nil {
		t.Fatalf("erro ao preparar cobrança expirada: %v", err)
	}

	if _, err := repository.ApplyWebhook(ctx, webhookFor(expired, prefix+"whk-expirada", ChargeApproved), ""); !errors.Is(err, ErrConflictingResult) {
		t.Fatalf("esperava ErrConflictingResult para cobrança expirada, recebeu %v", err)
	}

	for _, orderID := range []string{prefix + "atrasado", prefix + "expirada"} {
		found, _, _ := repository.FindChargeByOrder(ctx, orderID)
		if found.Status == ChargeApproved {
			t.Fatalf("webhook atrasado aprovou a cobrança %s", orderID)
		}
	}
}

func TestSaveEnvelopeConcurrentWriterGetsStoredEnvelope(t *testing.T) {
	repository, pool, prefix := openTestRepository(t)
	ctx := context.Background()
	charge := saved(t, repository, pendingCharge(t, pool, prefix+"1"))

	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(ctx)

	if _, err := first.Exec(ctx,
		"UPDATE pagamento.payment_charges SET result_envelope = $2 WHERE id = $1::uuid AND result_envelope IS NULL",
		charge.ID, `{"event_id":"primeiro"}`,
	); err != nil {
		t.Fatal(err)
	}

	type result struct {
		stored string
		err    error
	}
	done := make(chan result, 1)

	go func() {
		stored, err := repository.SaveResultEnvelope(ctx, charge.ID, `{"event_id":"segundo"}`)
		done <- result{stored, err}
	}()

	time.Sleep(300 * time.Millisecond)

	if err := first.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil || got.stored != `{"event_id":"primeiro"}` {
		t.Fatalf("segundo gravador deveria receber o envelope já gravado: %q %v", got.stored, got.err)
	}
}
