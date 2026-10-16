package mspagamento

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/webhook"
)

const webhookSecret = "segredo-do-webhook-com-mais-de-32-bytes"

type stubResultStore struct {
	stubOutboxStore

	outcome      WebhookOutcome
	applyErr     error
	applyCalls   int
	applied      WebhookEvent
	reason       string
	markedResult []string
}

func (store *stubResultStore) ApplyWebhook(
	_ context.Context,
	event WebhookEvent,
	reason string,
) (WebhookOutcome, error) {
	store.applyCalls++
	store.applied = event
	store.reason = reason

	return store.outcome, store.applyErr
}

func (store *stubResultStore) MarkResultPublished(_ context.Context, chargeID string) error {
	store.markedResult = append(store.markedResult, chargeID)

	return nil
}

func (store *stubResultStore) Ping(_ context.Context) error {
	return nil
}

const testChargeID = "d67af0cb-b7e8-44c7-a035-22c349c2cbc3"

func webhookJSON(status string) string {
	return `{"event_id":"whk-1","charge_id":"` + testChargeID +
		`","order_id":"PED-001","status":"` + status +
		`","occurred_at":"2026-10-01T18:10:00Z"}`
}

func decidedCharge(status string, reason string) Charge {
	return Charge{
		ID:          testChargeID,
		OrderID:     "PED-001",
		CustomerID:  "083a7808-462b-4df3-a755-3130bc571351",
		Amount:      59.80,
		Currency:    "BRL",
		Status:      status,
		Reason:      reason,
		ProcessedAt: time.Date(2026, 10, 1, 18, 10, 0, 0, time.UTC),
	}
}

func postWebhook(
	store ResultStore,
	publisher EventBus,
	body string,
	signature string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/payments", strings.NewReader(body))
	request.Header.Set(webhook.SignatureHeader, signature)

	recorder := httptest.NewRecorder()
	NewWebhookHandler(store, publisher, webhookSecret).Routes().ServeHTTP(recorder, request)

	return recorder
}

func signed(body string) string {
	return webhook.Sign(webhookSecret, []byte(body))
}

func TestWebhookApprovedPublishesPaymentApproved(t *testing.T) {
	store := &stubResultStore{outcome: WebhookOutcome{Charge: decidedCharge(ChargeApproved, "")}}
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("esperava 204, recebeu %d: %s", recorder.Code, recorder.Body.String())
	}

	if store.applied.EventID != "whk-1" || store.applied.Status != ChargeApproved || store.reason != "" {
		t.Fatalf("webhook aplicado inesperado: %+v %q", store.applied, store.reason)
	}

	if publisher.calls != 1 || publisher.eventType != events.PaymentApproved {
		t.Fatalf("publicação inesperada: %d %s", publisher.calls, publisher.eventType)
	}

	expected := map[string]any{
		"order_id":     "PED-001",
		"customer_id":  "083a7808-462b-4df3-a755-3130bc571351",
		"charge_id":    testChargeID,
		"amount":       59.80,
		"currency":     "BRL",
		"processed_at": "2026-10-01T18:10:00Z",
	}

	for key, value := range expected {
		if publisher.payload[key] != value {
			t.Fatalf("campo %s inesperado: %v", key, publisher.payload[key])
		}
	}

	if _, hasReason := publisher.payload["reason"]; hasReason {
		t.Fatal("aprovação não deve ter motivo")
	}

	if len(store.markedResult) != 1 {
		t.Fatal("publicação do resultado não foi marcada")
	}
}

func TestWebhookRefusedPublishesPaymentRefusedWithReason(t *testing.T) {
	store := &stubResultStore{outcome: WebhookOutcome{
		Charge: decidedCharge(ChargeRefused, RefusedReason),
	}}
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeRefused)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("esperava 204, recebeu %d", recorder.Code)
	}

	if store.reason != RefusedReason {
		t.Fatalf("motivo não enviado ao banco: %q", store.reason)
	}

	if publisher.eventType != events.PaymentRefused || publisher.payload["reason"] != RefusedReason {
		t.Fatalf("recusa inesperada: %s %+v", publisher.eventType, publisher.payload)
	}
}

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	store := &stubResultStore{}
	body := webhookJSON(ChargeApproved)

	for _, signature := range []string{"", "sha256=00", signed(webhookJSON(ChargeRefused))} {
		recorder := postWebhook(store, &stubPublisher{}, body, signature)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("esperava 401, recebeu %d", recorder.Code)
		}
	}

	if store.applyCalls != 0 {
		t.Fatal("webhook sem assinatura válida chegou ao banco")
	}
}

func TestWebhookRejectsInvalidBody(t *testing.T) {
	cases := map[string]string{
		"JSON inválido":     `{"event_id":`,
		"sem evento":        strings.Replace(webhookJSON(ChargeApproved), `"whk-1"`, `""`, 1),
		"cobrança não uuid": strings.Replace(webhookJSON(ChargeApproved), testChargeID, "abc", 1),
		"sem pedido":        strings.Replace(webhookJSON(ChargeApproved), `"PED-001"`, `""`, 1),
		"status inválido":   webhookJSON("PENDING"),
		"sem data":          strings.Replace(webhookJSON(ChargeApproved), `"2026-10-01T18:10:00Z"`, `"0001-01-01T00:00:00Z"`, 1),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			store := &stubResultStore{}
			recorder := postWebhook(store, &stubPublisher{}, body, signed(body))

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("esperava 400, recebeu %d", recorder.Code)
			}

			if store.applyCalls != 0 {
				t.Fatal("webhook inválido chegou ao banco")
			}
		})
	}
}

func TestWebhookMapsStoreErrors(t *testing.T) {
	cases := map[string]struct {
		err    error
		status int
	}{
		"cobrança inexistente":  {ErrChargeNotFound, http.StatusNotFound},
		"pedido divergente":     {ErrInvalidWebhook, http.StatusBadRequest},
		"resultado conflitante": {ErrConflictingResult, http.StatusConflict},
		"falha no banco":        {errors.New("banco fora do ar"), http.StatusInternalServerError},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			publisher := &stubPublisher{}
			body := webhookJSON(ChargeApproved)

			recorder := postWebhook(&stubResultStore{applyErr: testCase.err}, publisher, body, signed(body))
			if recorder.Code != testCase.status {
				t.Fatalf("esperava %d, recebeu %d", testCase.status, recorder.Code)
			}

			if publisher.calls != 0 {
				t.Fatal("erro gerou publicação")
			}
		})
	}
}

func TestWebhookDuplicateAlreadyPublishedDoesNotRepublish(t *testing.T) {
	charge := decidedCharge(ChargeApproved, "")
	charge.ResultPublished = true

	store := &stubResultStore{outcome: WebhookOutcome{Charge: charge, Duplicate: true}}
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code != http.StatusNoContent || publisher.calls != 0 {
		t.Fatalf("duplicado republicado: %d %d", recorder.Code, publisher.calls)
	}
}

func TestWebhookDuplicateNotYetPublishedIsPublished(t *testing.T) {
	store := &stubResultStore{outcome: WebhookOutcome{
		Charge:    decidedCharge(ChargeApproved, ""),
		Duplicate: true,
	}}
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code != http.StatusNoContent || publisher.calls != 1 {
		t.Fatalf("resultado pendente não publicado: %d %d", recorder.Code, publisher.calls)
	}
}

func TestWebhookPublishFailureKeepsResultPending(t *testing.T) {
	store := &stubResultStore{outcome: WebhookOutcome{Charge: decidedCharge(ChargeApproved, "")}}
	body := webhookJSON(ChargeApproved)

	// Resultado e envelope gravados: o webhook está processado e a rotina de
	// pendências reenvia o evento.
	recorder := postWebhook(store, &stubPublisher{err: errors.New("broker fora do ar")}, body, signed(body))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("esperava 204, recebeu %d", recorder.Code)
	}

	if len(store.resultEnvelopes) != 1 {
		t.Fatal("envelope do resultado não foi gravado")
	}

	if len(store.markedResult) != 0 {
		t.Fatal("resultado marcado como publicado sem publicação")
	}
}

func TestWebhookRejectsEventReusedForAnotherCharge(t *testing.T) {
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(&stubResultStore{applyErr: ErrWebhookEventReused}, publisher, body, signed(body))
	if recorder.Code != http.StatusConflict {
		t.Fatalf("esperava 409, recebeu %d", recorder.Code)
	}

	if publisher.calls != 0 {
		t.Fatal("evento reutilizado gerou publicação")
	}
}

func TestWebhookNeverPublishesUndecidedCharge(t *testing.T) {
	store := &stubResultStore{outcome: WebhookOutcome{
		Charge:    Charge{ID: testChargeID, OrderID: "PED-001", Status: ChargePending},
		Duplicate: true,
	}}
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code == http.StatusNoContent || publisher.calls != 0 {
		t.Fatalf("cobrança pendente publicada: %d %d", recorder.Code, publisher.calls)
	}
}

func TestWebhookResendsStoredResultEnvelope(t *testing.T) {
	charge := decidedCharge(ChargeApproved, "")
	charge.ResultEnvelope = storedPaymentEnvelope(t, "resultado-original", events.PaymentApproved)

	store := &stubResultStore{outcome: WebhookOutcome{Charge: charge, Duplicate: true}}
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("esperava 204, recebeu %d", recorder.Code)
	}

	if publisher.signs != 0 || len(publisher.published) != 1 || publisher.published[0].EventID != "resultado-original" {
		t.Fatalf("esperava reenviar o envelope gravado: assinaturas=%d %+v", publisher.signs, publisher.published)
	}
}

func TestWebhookEnvelopeStorageFailureAsksForRetry(t *testing.T) {
	store := &stubResultStore{outcome: WebhookOutcome{Charge: decidedCharge(ChargeApproved, "")}}
	store.saveEnvelopeErr = errors.New("banco fora do ar")
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(store, publisher, body, signed(body))
	if recorder.Code != http.StatusInternalServerError || publisher.calls != 0 {
		t.Fatalf("sem envelope gravado o Mock deve tentar de novo: %d %d", recorder.Code, publisher.calls)
	}
}

func TestWebhookAfterDeadlineGets409(t *testing.T) {
	publisher := &stubPublisher{}
	body := webhookJSON(ChargeApproved)

	recorder := postWebhook(&stubResultStore{applyErr: ErrCheckoutExpired}, publisher, body, signed(body))
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), `"code":"CHECKOUT_EXPIRED"`) {
		t.Fatalf("esperava 409 CHECKOUT_EXPIRED: %d %s", recorder.Code, recorder.Body.String())
	}

	if publisher.calls != 0 {
		t.Fatal("webhook após o prazo gerou publicação")
	}
}
