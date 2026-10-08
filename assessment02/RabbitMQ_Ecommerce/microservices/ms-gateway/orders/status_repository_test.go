package orders

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
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

		_, outboxErr := pool.Exec(cleanupCtx, `
			DELETE FROM gateway.event_outbox
			WHERE order_id IN (
				SELECT id FROM gateway.orders WHERE user_id = $1
			)
		`, userID)
		if outboxErr != nil {
			t.Errorf("erro ao limpar compensações de teste: %v", outboxErr)
			return
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

	var refusalOutboxCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM gateway.event_outbox
		WHERE order_id = $1
	`, order.DatabaseID).Scan(&refusalOutboxCount)
	if err != nil {
		t.Fatal(err)
	}
	if refusalOutboxCount != 1 {
		t.Fatalf("esperava uma compensação de recusa; recebeu %d", refusalOutboxCount)
	}

	var refusalEventType, refusalStatus string
	var refusalPayloadData []byte
	err = pool.QueryRow(ctx, `
		SELECT event_type, status, payload
		FROM gateway.event_outbox
		WHERE order_id = $1
	`, order.DatabaseID).Scan(
		&refusalEventType,
		&refusalStatus,
		&refusalPayloadData,
	)
	if err != nil {
		t.Fatal(err)
	}

	var refusalPayload events.WebOrderDeletedPayload
	if err := json.Unmarshal(refusalPayloadData, &refusalPayload); err != nil {
		t.Fatal(err)
	}
	if refusalEventType != events.OrderDeleted ||
		refusalStatus != "PENDENTE" ||
		refusalPayload.OrderID != order.ID ||
		refusalPayload.CustomerID != userID ||
		refusalPayload.Reason != events.ReasonPaymentRefused ||
		refusalPayload.CancelledAt.IsZero() {
		t.Fatal("compensação de recusa incorreta")
	}

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
	var outboxAttempts int
	var originalPreserved bool
	err = pool.QueryRow(ctx, `
		SELECT status, attempt_count, envelope = $2::jsonb
		FROM gateway.event_inbox
		WHERE event_id = $1
	`, envelope.EventID, mustMarshalEnvelope(t, envelope)).Scan(
		&inboxStatus,
		&outboxAttempts,
		&originalPreserved,
	)
	if err != nil {
		t.Fatal(err)
	}
	if inboxStatus != "RECEBIDO" || outboxAttempts != 0 || !originalPreserved {
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

	// O pedido atual já possui checkout: cancelamento deve ser conflito.
	cancelled, err := repository.Cancel(ctx, userID, order.ID)
	if cancelled || !errors.Is(err, ErrCancellationConflict) {
		t.Fatalf("cancelamento após checkout: cancelado=%v, erro=%v", cancelled, err)
	}
	checkState(StatusAwaitingPayment, 3)

	var blockedOutboxCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM gateway.event_outbox WHERE order_id = $1
	`, order.DatabaseID).Scan(&blockedOutboxCount)
	if err != nil {
		t.Fatal(err)
	}
	if blockedOutboxCount != 0 {
		t.Fatal("cancelamento proibido criou compensação")
	}

	for _, initialStatus := range []string{
		StatusPending,
		StatusStockConfirmed,
	} {
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

		historyBefore := 1
		if initialStatus == StatusStockConfirmed {
			payload.OrderID = order.ID
			applied, err = apply("estoque-para-cancelar-"+order.ID, payload)
			if err != nil || !applied {
				t.Fatalf("preparação do estoque: aplicado=%v, erro=%v", applied, err)
			}
			historyBefore = 2
		}

		cancelled, err = repository.Cancel(
			ctx,
			"00000000-0000-0000-0000-000000000001",
			order.ID,
		)
		if cancelled || !errors.Is(err, ErrOrderNotFound) {
			t.Fatalf("outro cliente: cancelado=%v, erro=%v", cancelled, err)
		}
		checkState(initialStatus, historyBefore)

		cancelled, err = repository.Cancel(ctx, userID, order.ID)
		if err != nil || !cancelled {
			t.Fatalf("cancelamento permitido: cancelado=%v, erro=%v", cancelled, err)
		}
		checkState(StatusCancelled, historyBefore+1)

		cancelled, err = repository.Cancel(ctx, userID, order.ID)
		if err != nil || cancelled {
			t.Fatalf("cancelamento repetido: cancelado=%v, erro=%v", cancelled, err)
		}
		checkState(StatusCancelled, historyBefore+1)

		var outboxCount int
		err = pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM gateway.event_outbox WHERE order_id = $1
		`, order.DatabaseID).Scan(&outboxCount)
		if err != nil {
			t.Fatal(err)
		}
		if outboxCount != 1 {
			t.Fatalf("esperava uma compensação; recebeu %d", outboxCount)
		}

		var eventType, outboxStatus string
		var payloadData []byte
		var unsigned bool
		err = pool.QueryRow(ctx, `
			SELECT event_type, status, payload, envelope IS NULL
			FROM gateway.event_outbox
			WHERE order_id = $1
		`, order.DatabaseID).Scan(
			&eventType, &outboxStatus, &payloadData, &unsigned,
		)
		if err != nil {
			t.Fatal(err)
		}

		var deletedPayload events.WebOrderDeletedPayload
		if err := json.Unmarshal(payloadData, &deletedPayload); err != nil {
			t.Fatal(err)
		}

		if eventType != events.OrderDeleted ||
			outboxStatus != "PENDENTE" ||
			!unsigned ||
			deletedPayload.OrderID != order.ID ||
			deletedPayload.CustomerID != userID ||
			deletedPayload.Reason != events.ReasonCustomerCancelled ||
			deletedPayload.CancelledAt.IsZero() {
			t.Fatal("compensação armazenada incorretamente")
		}
	}

	// Usa a compensação do último pedido cancelado neste teste.
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var outboxEventID string
	err = pool.QueryRow(ctx, `
		SELECT event_id::text
		FROM gateway.event_outbox
		WHERE order_id = $1 AND event_type = $2
	`, order.DatabaseID, events.OrderDeleted).Scan(&outboxEventID)
	if err != nil {
		t.Fatal(err)
	}

	firstEnvelope, err := repository.PrepareOutboxEnvelope(
		ctx, outboxEventID, privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}

	secondEnvelope, err := repository.PrepareOutboxEnvelope(
		ctx, outboxEventID, privateKey,
	)
	if err != nil {
		t.Fatal(err)
	}

	firstJSON := mustMarshalEnvelope(t, firstEnvelope)
	secondJSON := mustMarshalEnvelope(t, secondEnvelope)
	if firstJSON != secondJSON {
		t.Fatal("a segunda preparação alterou o envelope assinado")
	}

	if err := cryptography.VerifyEnvelope(
		secondEnvelope, &privateKey.PublicKey,
	); err != nil {
		t.Fatalf("assinatura inválida após releitura: %v", err)
	}

	var storedEnvelope, storedStatus string
	var attempts int
	err = pool.QueryRow(ctx, `
		SELECT envelope, status, attempt_count
		FROM gateway.event_outbox
		WHERE event_id = $1
	`, outboxEventID).Scan(
		&storedEnvelope, &storedStatus, &attempts,
	)
	if err != nil {
		t.Fatal(err)
	}

	if storedEnvelope != firstJSON {
		t.Fatal("o banco não preservou o texto do envelope")
	}
	if storedStatus != "PENDENTE" || attempts != 0 {
		t.Fatal("preparar o envelope alterou o estado de publicação")
	}

	checkState(StatusCancelled, 3)

	// Falha de publicação deve preservar o envelope e reagendar.
	simulatedError := errors.New("RabbitMQ indisponível no teste")
	if err := repository.ScheduleOutboxRetry(
		ctx, outboxEventID, simulatedError,
	); err != nil {
		t.Fatal(err)
	}

	var retryEnvelope, retryStatus, retryError string
	var retryAttempts int
	var outboxScheduledLater bool
	err = pool.QueryRow(ctx, `
		SELECT envelope, status, attempt_count, last_error,
			next_attempt_at > clock_timestamp()
		FROM gateway.event_outbox
		WHERE event_id = $1
	`, outboxEventID).Scan(
		&retryEnvelope, &retryStatus, &retryAttempts,
		&retryError, &outboxScheduledLater,
	)
	if err != nil {
		t.Fatal(err)
	}

	if retryEnvelope != firstJSON ||
		retryStatus != "PENDENTE" ||
		retryAttempts != 1 ||
		retryError != simulatedError.Error() ||
		!outboxScheduledLater {
		t.Fatal("falha não preservou e reagendou a compensação corretamente")
	}

	// Simula o registro posterior de uma confirmação positiva.
	if err := repository.MarkOutboxPublished(ctx, outboxEventID); err != nil {
		t.Fatal(err)
	}

	var publishedEnvelope, publishedStatus string
	var publicationAttempts int
	var hasPublishedAt, clearedError bool
	err = pool.QueryRow(ctx, `
		SELECT envelope, status, attempt_count,
			published_at IS NOT NULL, last_error IS NULL
		FROM gateway.event_outbox
		WHERE event_id = $1
	`, outboxEventID).Scan(
		&publishedEnvelope, &publishedStatus, &publicationAttempts,
		&hasPublishedAt, &clearedError,
	)
	if err != nil {
		t.Fatal(err)
	}

	if publishedEnvelope != firstJSON ||
		publishedStatus != "PUBLICADO" ||
		publicationAttempts != 2 ||
		!hasPublishedAt ||
		!clearedError {
		t.Fatal("publicação não foi registrada corretamente")
	}

	if err := repository.MarkOutboxPublished(
		ctx, outboxEventID,
	); !errors.Is(err, ErrOutboxNotPending) {
		t.Fatalf("publicação repetida: erro=%v", err)
	}

	if err := repository.ScheduleOutboxRetry(
		ctx, outboxEventID, simulatedError,
	); !errors.Is(err, ErrOutboxNotPending) {
		t.Fatalf("evento publicado aceitou reagendamento: erro=%v", err)
	}

	checkState(StatusCancelled, 3)

	t.Run("processamento da outbox", func(t *testing.T) {
		// Não permita que este teste processe eventos de outra conta.
		var foreignPending int
		err := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM gateway.event_outbox b
			JOIN gateway.orders o ON o.id = b.order_id
			WHERE b.status = 'PENDENTE' AND o.user_id <> $1
		`, userID).Scan(&foreignPending)
		if err != nil {
			t.Fatal(err)
		}
		if foreignPending != 0 {
			t.Fatal("há eventos pendentes de outra conta no banco de testes")
		}

		// Escolhe uma das compensações ainda pendentes desta conta.
		var targetID string
		err = pool.QueryRow(ctx, `
			SELECT b.event_id::text
			FROM gateway.event_outbox b
			JOIN gateway.orders o ON o.id = b.order_id
			WHERE b.status = 'PENDENTE' AND o.user_id = $1
			ORDER BY b.created_at, b.event_id
			LIMIT 1
		`, userID).Scan(&targetID)
		if err != nil {
			t.Fatal(err)
		}

		// Adia apenas as outras compensações criadas por este teste.
		_, err = pool.Exec(ctx, `
			UPDATE gateway.event_outbox b
			SET next_attempt_at = CASE
				WHEN b.event_id = $2::uuid
					THEN clock_timestamp() - INTERVAL '1 second'
				ELSE clock_timestamp() + INTERVAL '1 hour'
			END
			FROM gateway.orders o
			WHERE b.order_id = o.id
			AND o.user_id = $1
			AND b.status = 'PENDENTE'
		`, userID, targetID)
		if err != nil {
			t.Fatal(err)
		}

		var attemptedJSON string
		failure := errors.New("falha simulada de transporte")
		handled, err := repository.ProcessNextOutboxEvent(
			ctx, privateKey,
			func(_ context.Context, envelope events.EventEnvelope) error {
				if envelope.EventID != targetID {
					t.Fatal("selecionou outro evento")
				}
				if err := cryptography.VerifyEnvelope(
					envelope, &privateKey.PublicKey,
				); err != nil {
					t.Fatal(err)
				}
				attemptedJSON = mustMarshalEnvelope(t, envelope)
				return failure
			},
		)
		if !handled || !errors.Is(err, failure) {
			t.Fatalf("falha simulada: tratado=%v, erro=%v", handled, err)
		}

		var pendingStatus string
		var pendingAttempts int
		err = pool.QueryRow(ctx, `
			SELECT status, attempt_count
			FROM gateway.event_outbox WHERE event_id = $1
		`, targetID).Scan(&pendingStatus, &pendingAttempts)
		if err != nil {
			t.Fatal(err)
		}
		if pendingStatus != "PENDENTE" || pendingAttempts != 1 {
			t.Fatal("falha não manteve evento pendente com uma tentativa")
		}

		handled, err = repository.ProcessNextOutboxEvent(
			ctx, privateKey,
			func(context.Context, events.EventEnvelope) error {
				t.Fatal("publicador chamado antes do prazo")
				return nil
			},
		)
		if handled || err != nil {
			t.Fatalf("antes do prazo: tratado=%v, erro=%v", handled, err)
		}

		// Antecipa somente o evento escolhido, sem esperar dez segundos.
		_, err = pool.Exec(ctx, `
			UPDATE gateway.event_outbox
			SET next_attempt_at = clock_timestamp() - INTERVAL '1 second'
			WHERE event_id = $1
		`, targetID)
		if err != nil {
			t.Fatal(err)
		}

		handled, err = repository.ProcessNextOutboxEvent(
			ctx, privateKey,
			func(_ context.Context, envelope events.EventEnvelope) error {
				if mustMarshalEnvelope(t, envelope) != attemptedJSON {
					t.Fatal("nova tentativa alterou o envelope")
				}
				return nil
			},
		)
		if !handled || err != nil {
			t.Fatalf("sucesso simulado: tratado=%v, erro=%v", handled, err)
		}

		err = pool.QueryRow(ctx, `
			SELECT status, attempt_count
			FROM gateway.event_outbox WHERE event_id = $1
		`, targetID).Scan(&pendingStatus, &pendingAttempts)
		if err != nil {
			t.Fatal(err)
		}
		if pendingStatus != "PUBLICADO" || pendingAttempts != 2 {
			t.Fatal("sucesso não registrou publicação com duas tentativas")
		}
	})

	t.Run("limite de falhas da outbox", func(t *testing.T) {
		// Usa outra compensação ainda não processada desta conta de teste.
		var targetID string
		err := pool.QueryRow(ctx, `
			SELECT b.event_id::text
			FROM gateway.event_outbox b
			JOIN gateway.orders o ON o.id = b.order_id
			WHERE o.user_id = $1
			AND b.status = 'PENDENTE'
			AND b.attempt_count = 0
			ORDER BY b.created_at, b.event_id
			LIMIT 1
		`, userID).Scan(&targetID)
		if err != nil {
			t.Fatal(err)
		}

		envelope, err := repository.PrepareOutboxEnvelope(
			ctx, targetID, privateKey,
		)
		if err != nil {
			t.Fatal(err)
		}
		originalJSON := mustMarshalEnvelope(t, envelope)
		failure := errors.New("falha persistente simulada")

		for attempt := 1; attempt <= 12; attempt++ {
			if err := repository.ScheduleOutboxRetry(
				ctx, targetID, failure,
			); err != nil {
				t.Fatal(err)
			}

			var status, storedJSON, lastError string
			var attempts int
			var unpublished bool
			err := pool.QueryRow(ctx, `
				SELECT status, attempt_count, envelope, last_error,
					published_at IS NULL
				FROM gateway.event_outbox
				WHERE event_id = $1
			`, targetID).Scan(
				&status, &attempts, &storedJSON, &lastError, &unpublished,
			)
			if err != nil {
				t.Fatal(err)
			}

			expectedStatus := "PENDENTE"
			if attempt == 12 {
				expectedStatus = "PENDENTE_REVISAO"
			}

			if status != expectedStatus ||
				attempts != attempt ||
				storedJSON != originalJSON ||
				lastError != failure.Error() ||
				!unpublished {
				t.Fatalf("estado incorreto na tentativa %d", attempt)
			}
		}

		if err := repository.ScheduleOutboxRetry(
			ctx, targetID, failure,
		); !errors.Is(err, ErrOutboxNotPending) {
			t.Fatalf("evento em revisão aceitou nova tentativa: %v", err)
		}

		if err := repository.MarkOutboxPublished(
			ctx, targetID,
		); !errors.Is(err, ErrOutboxNotPending) {
			t.Fatalf("evento em revisão aceitou publicação: %v", err)
		}
	})
}

func mustMarshalEnvelope(t *testing.T, envelope events.EventEnvelope) string {
	t.Helper()
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
