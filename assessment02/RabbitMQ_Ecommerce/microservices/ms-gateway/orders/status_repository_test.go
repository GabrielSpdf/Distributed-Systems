package orders

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyStatusEventIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("defina TEST_DATABASE_URL para executar o teste de integração")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var databaseName string
	if err := pool.QueryRow(ctx, "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "ecommerce_events_test" {
		t.Fatal("execute este teste somente no banco ecommerce_events_test")
	}

	var userID string
	err = pool.QueryRow(ctx, `
		INSERT INTO gateway.users (name, email, password_hash)
		VALUES (
			'Teste de eventos',
			gen_random_uuid()::text || '@example.test',
			'conta-exclusiva-de-teste'
		)
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}

	// Remove somente os registros pertencentes à conta criada pelo teste.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 10*time.Second,
		)
		defer cleanupCancel()

		_, inboxErr := pool.Exec(cleanupCtx,
			"DELETE FROM gateway.event_inbox WHERE customer_id = $1",
			userID,
		)
		if inboxErr != nil {
			t.Errorf("erro ao limpar eventos de teste: %v", inboxErr)
		}

		_, err := pool.Exec(cleanupCtx,
			"DELETE FROM gateway.orders WHERE user_id = $1", userID,
		)
		if err != nil {
			t.Errorf("erro ao limpar pedidos de teste: %v", err)
			return
		}

		_, err = pool.Exec(cleanupCtx,
			"DELETE FROM gateway.users WHERE id = $1", userID,
		)
		if err != nil {
			t.Errorf("erro ao limpar usuário de teste: %v", err)
		}
	}()

	repository := NewRepository(pool)
	order, err := repository.Create(ctx, userID, []OrderItem{
		{
			ProductID:   "PROD-002",
			ProductName: "Detergente",
			Quantity:    1,
			UnitPrice:   4.9,
		},
	}, 490)
	if err != nil {
		t.Fatal(err)
	}

	checkState := func(expectedStatus string, expectedHistory int) {
		t.Helper()

		var status string
		var historyCount int
		err := pool.QueryRow(ctx, `
			SELECT o.status,
			       (SELECT COUNT(*)
			        FROM gateway.order_status_history h
			        WHERE h.order_id = o.id)
			FROM gateway.orders o
			WHERE o.id = $1
		`, order.DatabaseID).Scan(&status, &historyCount)
		if err != nil {
			t.Fatal(err)
		}

		if status != expectedStatus || historyCount != expectedHistory {
			t.Fatalf(
				"estado=%s, histórico=%d; esperado estado=%s, histórico=%d",
				status, historyCount, expectedStatus, expectedHistory,
			)
		}
	}

	payload := events.WebStockConfirmedPayload{
		OrderID:    order.ID,
		CustomerID: userID,
		Total:      4.9,
		Currency:   "BRL",
		ReservedAt: time.Now().UTC(),
	}

	apply := func(eventID string, value events.WebStockConfirmedPayload) (bool, error) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return repository.ApplyStatusEvent(
			ctx, eventID, events.OrderStockConfirmed, data,
		)
	}

	checkState(StatusPending, 1)

	wrongCustomer := payload
	wrongCustomer.CustomerID = "cliente-de-outro-pedido"
	applied, err := apply("teste-cliente-incorreto", wrongCustomer)
	if applied || !errors.Is(err, ErrInvalidStatusEvent) {
		t.Fatalf("cliente incorreto: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPending, 1)

	wrongTotal := payload
	wrongTotal.Total = 9.9
	applied, err = apply("teste-total-incorreto", wrongTotal)
	if applied || !errors.Is(err, ErrInvalidStatusEvent) {
		t.Fatalf("total incorreto: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPending, 1)

	applied, err = apply("teste-estoque-confirmado", payload)
	if err != nil || !applied {
		t.Fatalf("evento válido: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusStockConfirmed, 2)

	applied, err = apply("teste-estoque-confirmado", payload)
	if err != nil || applied {
		t.Fatalf("evento repetido: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusStockConfirmed, 2)

	var eventCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM gateway.order_status_history
		WHERE order_id = $1 AND event_id = $2
	`, order.DatabaseID, "teste-estoque-confirmado").Scan(&eventCount)
	if err != nil {
		t.Fatal(err)
	}
	if eventCount != 1 {
		t.Fatalf("esperava um registro do evento; recebeu %d", eventCount)
	}

	applyPayload := func(eventID, eventType string, value any) (bool, error) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return repository.ApplyStatusEvent(ctx, eventID, eventType, data)
	}

	payment := events.WebPaymentResultPayload{
		OrderID:     order.ID,
		CustomerID:  userID,
		ChargeID:    "COB-TESTE",
		Amount:      4.9,
		Currency:    "BRL",
		ProcessedAt: time.Now().UTC(),
	}

	// Pagamento não pode ser aprovado antes do checkout.
	applied, err = applyPayload(
		"teste-pagamento-antecipado", events.PaymentApproved, payment,
	)
	if applied || !errors.Is(err, ErrStatusTransition) {
		t.Fatalf("pagamento antecipado: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusStockConfirmed, 2)

	checkout := events.WebPaymentCheckoutPayload{
		OrderID:     order.ID,
		CustomerID:  userID,
		ChargeID:    "COB-TESTE",
		CheckoutURL: "https://checkout.example.com/COB-TESTE",
		ExpiresAt:   time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond),
	}

	applied, err = applyPayload(
		"teste-checkout", events.PaymentCheckoutAvailable, checkout,
	)
	if err != nil || !applied {
		t.Fatalf("checkout: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusAwaitingPayment, 3)

	var savedURL, savedChargeID string
	var savedExpiry time.Time
	err = pool.QueryRow(ctx, `
		SELECT checkout_url, charge_id, checkout_expires_at
		FROM gateway.orders WHERE id = $1
	`, order.DatabaseID).Scan(&savedURL, &savedChargeID, &savedExpiry)
	if err != nil {
		t.Fatal(err)
	}
	if savedURL != checkout.CheckoutURL ||
		savedChargeID != checkout.ChargeID ||
		!savedExpiry.Equal(checkout.ExpiresAt) {
		t.Fatal("dados do checkout não foram persistidos corretamente")
	}

	wrongCharge := payment
	wrongCharge.ChargeID = "COB-OUTRA"
	applied, err = applyPayload(
		"teste-cobranca-incorreta", events.PaymentApproved, wrongCharge,
	)
	if applied || !errors.Is(err, ErrInvalidStatusEvent) {
		t.Fatalf("cobrança incorreta: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusAwaitingPayment, 3)

	wrongAmount := payment
	wrongAmount.Amount = 9.9
	applied, err = applyPayload(
		"teste-pagamento-valor-incorreto", events.PaymentApproved, wrongAmount,
	)
	if applied || !errors.Is(err, ErrInvalidStatusEvent) {
		t.Fatalf("valor incorreto: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusAwaitingPayment, 3)

	applied, err = applyPayload(
		"teste-pagamento-aprovado", events.PaymentApproved, payment,
	)
	if err != nil || !applied {
		t.Fatalf("pagamento aprovado: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPaymentApproved, 4)

	shipped := events.WebOrderShippedPayload{
		OrderID:      order.ID,
		CustomerID:   userID,
		InvoiceID:    "NF-TESTE",
		TrackingCode: "RASTREIO-TESTE",
		ShippedAt:    time.Now().UTC().Truncate(time.Microsecond),
	}

	applied, err = applyPayload(
		"teste-pedido-enviado", events.OrderShipped, shipped,
	)
	if err != nil || !applied {
		t.Fatalf("pedido enviado: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusShipped, 5)

	var savedInvoice, savedTracking string
	var savedShippedAt time.Time
	err = pool.QueryRow(ctx, `
		SELECT invoice_id, tracking_code, shipped_at
		FROM gateway.orders WHERE id = $1
	`, order.DatabaseID).Scan(&savedInvoice, &savedTracking, &savedShippedAt)
	if err != nil {
		t.Fatal(err)
	}
	if savedInvoice != shipped.InvoiceID ||
		savedTracking != shipped.TrackingCode ||
		!savedShippedAt.Equal(shipped.ShippedAt) {
		t.Fatal("dados do envio não foram persistidos corretamente")
	}

	applied, err = applyPayload(
		"teste-pedido-enviado", events.OrderShipped, shipped,
	)
	if err != nil || applied {
		t.Fatalf("envio repetido: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusShipped, 5)

	// Novo pedido para testar estoque indisponível.
	order, err = repository.Create(ctx, userID, []OrderItem{
		{
			ProductID:   "PROD-002",
			ProductName: "Detergente",
			Quantity:    1,
			UnitPrice:   4.9,
		},
	}, 490)
	if err != nil {
		t.Fatal(err)
	}

	unavailable := events.WebStockUnavailablePayload{
		OrderID:    order.ID,
		CustomerID: userID,
		Reason:     "Estoque insuficiente",
		UnavailableItems: []events.WebUnavailableItem{
			{
				ProductID:         "PROD-OUTRO",
				RequestedQuantity: 1,
				AvailableQuantity: 0,
			},
		},
	}

	applied, err = applyPayload(
		"teste-item-incorreto", events.StockUnavailable, unavailable,
	)
	if applied || !errors.Is(err, ErrInvalidStatusEvent) {
		t.Fatalf("item incorreto: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPending, 1)

	unavailable.UnavailableItems[0].ProductID = "PROD-002"
	applied, err = applyPayload(
		"teste-estoque-indisponivel", events.StockUnavailable, unavailable,
	)
	if err != nil || !applied {
		t.Fatalf("estoque indisponível: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusStockUnavailable, 2)

	// Outro pedido para testar pagamento recusado.
	order, err = repository.Create(ctx, userID, []OrderItem{
		{
			ProductID:   "PROD-002",
			ProductName: "Detergente",
			Quantity:    1,
			UnitPrice:   4.9,
		},
	}, 490)
	if err != nil {
		t.Fatal(err)
	}

	payload.OrderID = order.ID
	applied, err = apply("teste-estoque-para-recusa", payload)
	if err != nil || !applied {
		t.Fatalf("reserva para recusa: aplicado=%v, erro=%v", applied, err)
	}
	checkout.OrderID = order.ID
	applied, err = applyPayload(
		"teste-checkout-para-recusa", events.PaymentCheckoutAvailable, checkout,
	)
	if err != nil || !applied {
		t.Fatalf("checkout para recusa: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusAwaitingPayment, 3)

	payment.OrderID = order.ID
	payment.Reason = "Pagamento recusado"
	applied, err = applyPayload(
		"teste-pagamento-recusado", events.PaymentRefused, payment,
	)
	if err != nil || !applied {
		t.Fatalf("pagamento recusado: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPaymentRefused, 4)

	applied, err = applyPayload(
		"teste-pagamento-recusado", events.PaymentRefused, payment,
	)
	if err != nil || applied {
		t.Fatalf("recusa repetida: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPaymentRefused, 4)

	applied, err = applyPayload(
		"teste-aprovacao-apos-recusa", events.PaymentApproved, payment,
	)
	if applied || !errors.Is(err, ErrStatusTransition) {
		t.Fatalf("aprovação após recusa: aplicado=%v, erro=%v", applied, err)
	}
	checkState(StatusPaymentRefused, 4)

	// Testa somente o armazenamento, sem aplicar a atualização.
	inboxPayload := events.WebStockConfirmedPayload{
		OrderID:    order.ID,
		CustomerID: userID,
		Total:      4.9,
		Currency:   "BRL",
		ReservedAt: time.Now().UTC(),
	}
	inboxData, err := json.Marshal(inboxPayload)
	if err != nil {
		t.Fatal(err)
	}

	envelope := events.EventEnvelope{
		EventID:   "teste-inbox-" + order.ID,
		EventType: events.OrderStockConfirmed,
		Producer:  events.ProducerStock,
		Timestamp: time.Now().UTC(),
		Payload:   inboxData,
		Signature: "assinatura-ficticia-para-teste-do-repositorio",
	}

	stored, err := repository.StoreStatusEvent(ctx, envelope)
	if err != nil || !stored {
		t.Fatalf("evento novo: armazenado=%v, erro=%v", stored, err)
	}

	stored, err = repository.StoreStatusEvent(ctx, envelope)
	if err != nil || stored {
		t.Fatalf("evento repetido: armazenado=%v, erro=%v", stored, err)
	}

	// Mesmo identificador, mas conteúdo diferente: deve rejeitar.
	changedEnvelope := envelope
	inboxPayload.Total = 9.9
	changedEnvelope.Payload, err = json.Marshal(inboxPayload)
	if err != nil {
		t.Fatal(err)
	}

	stored, err = repository.StoreStatusEvent(ctx, changedEnvelope)
	if stored || !errors.Is(err, ErrInvalidStatusEvent) {
		t.Fatalf("colisão de identificador: armazenado=%v, erro=%v", stored, err)
	}

	var inboxCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM gateway.event_inbox
		WHERE customer_id = $1
	`, userID).Scan(&inboxCount)
	if err != nil {
		t.Fatal(err)
	}
	if inboxCount != 1 {
		t.Fatalf("esperava um evento armazenado; recebeu %d", inboxCount)
	}

	var inboxStatus string
	var attempts int
	var originalPreserved bool
	err = pool.QueryRow(ctx, `
		SELECT status, attempt_count, envelope = $2::jsonb
		FROM gateway.event_inbox
		WHERE event_id = $1
	`, envelope.EventID, mustMarshalEnvelope(t, envelope)).Scan(
		&inboxStatus,
		&attempts,
		&originalPreserved,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inboxStatus != "RECEBIDO" || attempts != 0 || !originalPreserved {
		t.Fatal("estado inicial ou envelope original incorreto")
	}

	// Guardar na inbox não pode alterar o pedido.
	checkState(StatusPaymentRefused, 4)

	processNext := func() {
		t.Helper()
		handled, err := repository.ProcessNextStatusEvent(ctx)
		if err != nil || !handled {
			t.Fatalf("processamento: tratado=%v, erro=%v", handled, err)
		}
	}

	checkInbox := func(eventID, expectedStatus string, expectedAttempts int) {
		t.Helper()
		var status string
		var count int
		err := pool.QueryRow(ctx, `
			SELECT status, attempt_count
			FROM gateway.event_inbox WHERE event_id = $1
		`, eventID).Scan(&status, &count)
		if err != nil {
			t.Fatal(err)
		}
		if status != expectedStatus || count != expectedAttempts {
			t.Fatalf("inbox: estado=%s, tentativas=%d; esperado %s/%d",
				status, count, expectedStatus, expectedAttempts)
		}
	}

	// O evento armazenado anteriormente não pode alterar um pedido recusado.
	processNext()
	checkInbox(envelope.EventID, "RECEBIDO", 1)
	checkState(StatusPaymentRefused, 4)

	var scheduledLater bool
	err = pool.QueryRow(ctx, `
		SELECT next_attempt_at > clock_timestamp()
		FROM gateway.event_inbox WHERE event_id = $1
	`, envelope.EventID).Scan(&scheduledLater)
	if err != nil {
		t.Fatal(err)
	}
	if !scheduledLater {
		t.Fatal("evento não foi reagendado para o futuro")
	}

	handled, err := repository.ProcessNextStatusEvent(ctx)
	if err != nil || handled {
		t.Fatalf("evento reagendado não deveria ser tratado agora: %v/%v",
			handled, err)
	}

	// Simula a última tentativa, sem esperar dois minutos.
	_, err = pool.Exec(ctx, `
		UPDATE gateway.event_inbox
		SET attempt_count = 11,
		    next_attempt_at = clock_timestamp() - INTERVAL '1 second'
		WHERE event_id = $1
	`, envelope.EventID)
	if err != nil {
		t.Fatal(err)
	}
	processNext()
	checkInbox(envelope.EventID, "PENDENTE_REVISAO", 12)
	checkState(StatusPaymentRefused, 4)

	// Novo pedido: checkout chega antes da confirmação do estoque.
	order, err = repository.Create(ctx, userID, []OrderItem{
		{
			ProductID:   "PROD-002",
			ProductName: "Detergente",
			Quantity:    1,
			UnitPrice:   4.9,
		},
	}, 490)
	if err != nil {
		t.Fatal(err)
	}

	storeForProcessing := func(eventID, eventType, producer string, value any) {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := repository.StoreStatusEvent(ctx, events.EventEnvelope{
			EventID:   eventID,
			EventType: eventType,
			Producer:  producer,
			Timestamp: time.Now().UTC(),
			Payload:   data,
			Signature: "assinatura-ficticia-para-teste-do-repositorio",
		})
		if err != nil || !stored {
			t.Fatalf("armazenamento: novo=%v, erro=%v", stored, err)
		}
	}

	checkout.OrderID = order.ID
	checkoutEventID := "checkout-antecipado-" + order.ID
	storeForProcessing(
		checkoutEventID,
		events.PaymentCheckoutAvailable,
		events.ProducerPayment,
		checkout,
	)
	processNext()
	checkInbox(checkoutEventID, "RECEBIDO", 1)
	checkState(StatusPending, 1)

	payload.OrderID = order.ID
	stockEventID := "estoque-inbox-" + order.ID
	storeForProcessing(
		stockEventID,
		events.OrderStockConfirmed,
		events.ProducerStock,
		payload,
	)
	processNext()
	checkInbox(stockEventID, "PROCESSADO", 1)
	checkState(StatusStockConfirmed, 2)

	// Antecipa apenas o evento deste teste para evitar espera.
	_, err = pool.Exec(ctx, `
		UPDATE gateway.event_inbox
		SET next_attempt_at = clock_timestamp() - INTERVAL '1 second'
		WHERE event_id = $1
	`, checkoutEventID)
	if err != nil {
		t.Fatal(err)
	}
	processNext()
	checkInbox(checkoutEventID, "PROCESSADO", 2)
	checkState(StatusAwaitingPayment, 3)
}

func mustMarshalEnvelope(t *testing.T, envelope events.EventEnvelope) string {
	t.Helper()
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
