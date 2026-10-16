package msestoque

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

type stockFixture struct {
	category string
	productA string
	productB string
	soldOut  string
	orders   string
}

func openTestRepository(t *testing.T) (*Repository, *pgxpool.Pool) {
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

	return NewRepository(pool), pool
}

func seedStock(t *testing.T, pool *pgxpool.Pool) stockFixture {
	t.Helper()

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)

	fixture := stockFixture{
		category: "teste-" + suffix,
		productA: "TEST-" + suffix + "-A",
		productB: "TEST-" + suffix + "-B",
		soldOut:  "TEST-" + suffix + "-C",
		orders:   "TEST-ORD-" + suffix + "-",
	}

	ctx := context.Background()

	const insertProduct = `
		INSERT INTO estoque.products (id, name, category, price, quantity)
		VALUES ($1, $2, $3, $4::numeric, $5)
	`

	rows := []struct {
		id       string
		name     string
		price    string
		quantity int
	}{
		{fixture.productA, "Produto A", "10.00", 5},
		{fixture.productB, "Produto B", "2.50", 2},
		{fixture.soldOut, "Produto C", "1.00", 0},
	}

	for _, row := range rows {
		if _, err := pool.Exec(
			ctx,
			insertProduct,
			row.id,
			row.name,
			fixture.category,
			row.price,
			row.quantity,
		); err != nil {
			t.Fatalf("erro ao criar produto de teste: %v", err)
		}
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(
			ctx,
			"DELETE FROM estoque.stock_reservations WHERE order_id LIKE $1",
			fixture.orders+"%",
		)
		_, _ = pool.Exec(
			ctx,
			"DELETE FROM estoque.products WHERE category = $1",
			fixture.category,
		)
	})

	return fixture
}

func quantityOf(t *testing.T, pool *pgxpool.Pool, productID string) int {
	t.Helper()

	var quantity int

	err := pool.QueryRow(
		context.Background(),
		"SELECT quantity FROM estoque.products WHERE id = $1",
		productID,
	).Scan(&quantity)
	if err != nil {
		t.Fatalf("erro ao consultar quantidade de %s: %v", productID, err)
	}

	return quantity
}

func TestListProductsFiltersByCategoryAndAvailability(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()

	all, err := repository.ListProducts(ctx, ProductFilter{Category: fixture.category})
	if err != nil {
		t.Fatalf("erro ao listar: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("esperava 3 produtos, recebeu %d", len(all))
	}

	if all[0].ID != fixture.productA || all[0].Price != 10.00 || all[0].Quantity != 5 {
		t.Fatalf("primeiro produto inesperado: %+v", all[0])
	}

	available := true

	inStock, err := repository.ListProducts(ctx, ProductFilter{
		Category:  fixture.category,
		Available: &available,
	})
	if err != nil {
		t.Fatalf("erro ao listar disponíveis: %v", err)
	}

	if len(inStock) != 2 {
		t.Fatalf("esperava 2 produtos disponíveis, recebeu %d", len(inStock))
	}

	unavailable := false

	soldOut, err := repository.ListProducts(ctx, ProductFilter{
		Category:  fixture.category,
		Available: &unavailable,
	})
	if err != nil {
		t.Fatalf("erro ao listar esgotados: %v", err)
	}

	if len(soldOut) != 1 || soldOut[0].ID != fixture.soldOut {
		t.Fatalf("esperava apenas o produto esgotado, recebeu %+v", soldOut)
	}

	none, err := repository.ListProducts(ctx, ProductFilter{Category: "inexistente-" + fixture.category})
	if err != nil {
		t.Fatalf("erro ao listar categoria vazia: %v", err)
	}

	if none == nil || len(none) != 0 {
		t.Fatalf("esperava lista vazia não nula, recebeu %#v", none)
	}
}

func TestReserveDecrementsStockOnlyOnce(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "1"

	items := []ReservationItem{
		{ProductID: fixture.productA, Quantity: 2},
		{ProductID: fixture.productB, Quantity: 1},
		{ProductID: fixture.productA, Quantity: 1},
	}

	first, err := repository.Reserve(ctx, orderID, items)
	if err != nil {
		t.Fatalf("erro ao reservar: %v", err)
	}

	if first.Status != ReservationReserved || first.ReservedAt.IsZero() {
		t.Fatalf("resultado inesperado: %+v", first)
	}

	if quantityOf(t, pool, fixture.productA) != 2 || quantityOf(t, pool, fixture.productB) != 1 {
		t.Fatal("estoque não foi baixado corretamente")
	}

	second, err := repository.Reserve(ctx, orderID, items)
	if err != nil {
		t.Fatalf("erro na reserva repetida: %v", err)
	}

	if second.Status != ReservationReserved || !second.ReservedAt.Equal(first.ReservedAt) {
		t.Fatalf("reserva repetida inesperada: %+v", second)
	}

	if quantityOf(t, pool, fixture.productA) != 2 || quantityOf(t, pool, fixture.productB) != 1 {
		t.Fatal("reserva repetida baixou o estoque novamente")
	}
}

func TestReserveWithInsufficientStockChangesNothing(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "2"

	result, err := repository.Reserve(ctx, orderID, []ReservationItem{
		{ProductID: fixture.productA, Quantity: 1},
		{ProductID: fixture.productB, Quantity: 3},
	})
	if err != nil {
		t.Fatalf("erro ao reservar: %v", err)
	}

	if result.Status != ReservationUnavailable {
		t.Fatalf("esperava UNAVAILABLE, recebeu %s", result.Status)
	}

	if len(result.Shortfalls) != 1 ||
		result.Shortfalls[0].ProductID != fixture.productB ||
		result.Shortfalls[0].RequestedQuantity != 3 ||
		result.Shortfalls[0].AvailableQuantity != 2 {
		t.Fatalf("faltas inesperadas: %+v", result.Shortfalls)
	}

	if quantityOf(t, pool, fixture.productA) != 5 || quantityOf(t, pool, fixture.productB) != 2 {
		t.Fatal("houve reserva parcial")
	}

	again, err := repository.Reserve(ctx, orderID, []ReservationItem{
		{ProductID: fixture.productA, Quantity: 1},
	})
	if err != nil {
		t.Fatalf("erro na tentativa repetida: %v", err)
	}

	if again.Status != ReservationUnavailable {
		t.Fatalf("pedido indisponível foi reservado depois: %s", again.Status)
	}

	if quantityOf(t, pool, fixture.productA) != 5 {
		t.Fatal("tentativa repetida alterou o estoque")
	}
}

func TestReleaseGivesStockBackOnlyOnce(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "3"

	if _, err := repository.Reserve(ctx, orderID, []ReservationItem{
		{ProductID: fixture.productA, Quantity: 4},
	}); err != nil {
		t.Fatalf("erro ao reservar: %v", err)
	}

	status, err := repository.Release(ctx, orderID)
	if err != nil {
		t.Fatalf("erro ao liberar: %v", err)
	}

	if status != ReservationReleased {
		t.Fatalf("esperava RELEASED, recebeu %s", status)
	}

	if quantityOf(t, pool, fixture.productA) != 5 {
		t.Fatal("estoque não foi devolvido")
	}

	if _, err := repository.Release(ctx, orderID); err != nil {
		t.Fatalf("erro na liberação repetida: %v", err)
	}

	if quantityOf(t, pool, fixture.productA) != 5 {
		t.Fatal("liberação repetida devolveu o estoque duas vezes")
	}
}

func TestReleaseBeforeReserveBlocksLaterReservation(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "4"

	status, err := repository.Release(ctx, orderID)
	if err != nil {
		t.Fatalf("erro no cancelamento preventivo: %v", err)
	}

	if status != ReservationReleased {
		t.Fatalf("esperava RELEASED, recebeu %s", status)
	}

	result, err := repository.Reserve(ctx, orderID, []ReservationItem{
		{ProductID: fixture.productA, Quantity: 1},
	})
	if err != nil {
		t.Fatalf("erro ao reservar pedido cancelado: %v", err)
	}

	if result.Status != ReservationReleased {
		t.Fatalf("pedido cancelado foi reservado: %s", result.Status)
	}

	if quantityOf(t, pool, fixture.productA) != 5 {
		t.Fatal("pedido cancelado baixou o estoque")
	}
}

func TestReserveRejectsInvalidOrder(t *testing.T) {
	repository, _ := openTestRepository(t)

	_, err := repository.Reserve(context.Background(), "", []ReservationItem{
		{ProductID: "PROD-001", Quantity: 1},
	})
	if !errors.Is(err, ErrInvalidOrder) {
		t.Fatalf("esperava ErrInvalidOrder, recebeu %v", err)
	}
}

func TestReserveKeepsOriginalShortfallsAfterRestock(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "5"
	items := []ReservationItem{{ProductID: fixture.productB, Quantity: 3}}

	if _, err := repository.Reserve(ctx, orderID, items); err != nil {
		t.Fatalf("erro ao reservar: %v", err)
	}

	if _, err := pool.Exec(ctx, "UPDATE estoque.products SET quantity = 10 WHERE id = $1", fixture.productB); err != nil {
		t.Fatalf("erro ao repor estoque: %v", err)
	}

	again, err := repository.Reserve(ctx, orderID, items)
	if err != nil {
		t.Fatalf("erro na repetição: %v", err)
	}

	if again.Status != ReservationUnavailable || len(again.Shortfalls) != 1 ||
		again.Shortfalls[0].RequestedQuantity != 3 || again.Shortfalls[0].AvailableQuantity != 2 {
		t.Fatalf("resultado original não foi preservado após reposição: %+v", again)
	}

	if quantityOf(t, pool, fixture.productB) != 10 {
		t.Fatal("repetição de pedido indisponível alterou o estoque")
	}
}

func TestResultEnvelopeFirstWriterWins(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "6"
	items := []ReservationItem{{ProductID: fixture.productA, Quantity: 1}}

	if _, err := repository.Reserve(ctx, orderID, items); err != nil {
		t.Fatalf("erro ao reservar: %v", err)
	}

	stored, err := repository.SaveResultEnvelope(ctx, orderID, `{"event_id":"primeiro"}`)
	if err != nil || stored != `{"event_id":"primeiro"}` {
		t.Fatalf("primeiro envelope não gravado: %q %v", stored, err)
	}

	stored, err = repository.SaveResultEnvelope(ctx, orderID, `{"event_id":"segundo"}`)
	if err != nil || stored != `{"event_id":"primeiro"}` {
		t.Fatalf("segundo envelope substituiu o primeiro: %q %v", stored, err)
	}

	again, err := repository.Reserve(ctx, orderID, items)
	if err != nil || again.Envelope != `{"event_id":"primeiro"}` || again.Published {
		t.Fatalf("reserva repetida sem o envelope gravado: %+v %v", again, err)
	}
}

func TestPendingResultsUntilPublished(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	reserved := fixture.orders + "7"
	released := fixture.orders + "8"
	items := []ReservationItem{{ProductID: fixture.productA, Quantity: 1}}

	for _, orderID := range []string{reserved, released} {
		if _, err := repository.Reserve(ctx, orderID, items); err != nil {
			t.Fatalf("erro ao reservar: %v", err)
		}
		if _, err := repository.SaveResultEnvelope(ctx, orderID, `{"event_id":"`+orderID+`"}`); err != nil {
			t.Fatalf("erro ao gravar envelope: %v", err)
		}
	}

	if _, err := repository.Release(ctx, released); err != nil {
		t.Fatalf("erro ao liberar: %v", err)
	}

	isPending := func(orderID string) bool {
		pending, err := repository.ListPendingResults(ctx, 0, 1000)
		if err != nil {
			t.Fatalf("erro ao listar pendências: %v", err)
		}
		for _, result := range pending {
			if result.OrderID == orderID {
				return true
			}
		}
		return false
	}

	if !isPending(reserved) {
		t.Fatal("resultado não publicado não aparece como pendente")
	}

	if isPending(released) {
		t.Fatal("pedido já liberado não deve ter resultado reenviado")
	}

	if err := repository.MarkResultPublished(ctx, reserved); err != nil {
		t.Fatalf("erro ao marcar publicação: %v", err)
	}

	if isPending(reserved) {
		t.Fatal("resultado publicado continua pendente")
	}

	again, err := repository.Reserve(ctx, reserved, items)
	if err != nil || !again.Published {
		t.Fatalf("reserva repetida não indica publicação: %+v %v", again, err)
	}
}

func TestSaveResultEnvelopeConcurrentWriterGetsStoredEnvelope(t *testing.T) {
	repository, pool := openTestRepository(t)
	fixture := seedStock(t, pool)
	ctx := context.Background()
	orderID := fixture.orders + "9"

	if _, err := repository.Reserve(ctx, orderID, []ReservationItem{{ProductID: fixture.productA, Quantity: 1}}); err != nil {
		t.Fatalf("erro ao reservar: %v", err)
	}

	// Primeiro gravador segura a linha sem confirmar.
	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Rollback(ctx)

	if _, err := first.Exec(ctx,
		"UPDATE estoque.stock_reservations SET result_envelope = $2 WHERE order_id = $1 AND result_envelope IS NULL",
		orderID, `{"event_id":"primeiro"}`,
	); err != nil {
		t.Fatal(err)
	}

	type result struct {
		stored string
		err    error
	}
	done := make(chan result, 1)

	go func() {
		stored, err := repository.SaveResultEnvelope(ctx, orderID, `{"event_id":"segundo"}`)
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
