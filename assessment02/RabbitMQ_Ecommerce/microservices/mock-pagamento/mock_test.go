package mockpagamento

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/webhook"
)

const testSecret = "segredo-do-webhook-com-mais-de-32-bytes"

type webhookReceiver struct {
	mu         sync.Mutex
	bodies     []string
	signatures []string
	statuses   []int
	server     *httptest.Server
}

// newWebhookReceiver answers with the given statuses in order; the last one
// repeats.
func newWebhookReceiver(t *testing.T, statuses ...int) *webhookReceiver {
	t.Helper()

	receiver := &webhookReceiver{statuses: statuses}
	receiver.server = httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			body, _ := io.ReadAll(request.Body)

			receiver.mu.Lock()
			receiver.bodies = append(receiver.bodies, string(body))
			receiver.signatures = append(
				receiver.signatures,
				request.Header.Get(webhook.SignatureHeader),
			)
			index := len(receiver.bodies) - 1
			if index >= len(receiver.statuses) {
				index = len(receiver.statuses) - 1
			}
			status := receiver.statuses[index]
			receiver.mu.Unlock()

			writer.WriteHeader(status)
		},
	))
	t.Cleanup(receiver.server.Close)

	return receiver
}

func (receiver *webhookReceiver) calls() int {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()

	return len(receiver.bodies)
}

type testMock struct {
	mock  *Mock
	now   time.Time
	route http.Handler
}

func newTestMock(t *testing.T) *testMock {
	t.Helper()

	instance := &testMock{
		now: time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC),
	}

	instance.mock = NewMock(Options{
		PublicURL:     "http://localhost:8083",
		WebhookSecret: testSecret,
		CheckoutTTL:   30 * time.Minute,
		Now:           func() time.Time { return instance.now },
	})
	instance.route = instance.mock.Routes()

	return instance
}

func (instance *testMock) do(
	method string,
	target string,
	body string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))

	for name, value := range headers {
		request.Header.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	instance.route.ServeHTTP(recorder, request)

	return recorder
}

func chargeBody(orderID string, amount string, webhookURL string) string {
	return `{"order_id":"` + orderID + `","amount":` + amount +
		`,"currency":"BRL","webhook_url":"` + webhookURL + `"}`
}

func (instance *testMock) createCharge(
	t *testing.T,
	orderID string,
	webhookURL string,
) ChargeResponse {
	t.Helper()

	recorder := instance.do(
		http.MethodPost,
		"/charges",
		chargeBody(orderID, "59.80", webhookURL),
		map[string]string{"Idempotency-Key": orderID},
	)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("esperava 201, recebeu %d: %s", recorder.Code, recorder.Body.String())
	}

	var response ChargeResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}

	return response
}

func TestCreateChargeReturnsCheckout(t *testing.T) {
	instance := newTestMock(t)

	charge := instance.createCharge(t, "PED-001", "http://localhost:8082/webhooks/payments")

	if charge.ChargeID == "" || charge.OrderID != "PED-001" || charge.Status != StatusPending {
		t.Fatalf("cobrança inesperada: %+v", charge)
	}

	if charge.CheckoutURL != "http://localhost:8083/checkout/"+charge.ChargeID {
		t.Fatalf("checkout_url inesperada: %s", charge.CheckoutURL)
	}

	if !charge.ExpiresAt.Equal(instance.now.Add(30 * time.Minute)) {
		t.Fatalf("expires_at inesperado: %s", charge.ExpiresAt)
	}
}

func TestCreateChargeIsIdempotent(t *testing.T) {
	instance := newTestMock(t)
	first := instance.createCharge(t, "PED-001", "http://localhost:8082/webhooks/payments")

	recorder := instance.do(
		http.MethodPost,
		"/charges",
		chargeBody("PED-001", "59.80", "http://localhost:8082/webhooks/payments"),
		map[string]string{"Idempotency-Key": "PED-001"},
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("esperava 200 na repetição, recebeu %d", recorder.Code)
	}

	var second ChargeResponse
	if err := json.NewDecoder(recorder.Body).Decode(&second); err != nil {
		t.Fatalf("resposta inválida: %v", err)
	}

	if second.ChargeID != first.ChargeID {
		t.Fatalf("repetição criou outra cobrança: %s != %s", second.ChargeID, first.ChargeID)
	}
}

func TestCreateChargeRejectsReusedKeyWithOtherBody(t *testing.T) {
	instance := newTestMock(t)
	instance.createCharge(t, "PED-001", "http://localhost:8082/webhooks/payments")

	recorder := instance.do(
		http.MethodPost,
		"/charges",
		chargeBody("PED-001", "10.00", "http://localhost:8082/webhooks/payments"),
		map[string]string{"Idempotency-Key": "PED-001"},
	)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("esperava 409, recebeu %d", recorder.Code)
	}
}

func TestCreateChargeRejectsInvalidBody(t *testing.T) {
	cases := map[string]string{
		"JSON inválido":    `{"order_id":`,
		"sem pedido":       chargeBody("", "59.80", "http://localhost:8082/webhooks/payments"),
		"valor zero":       chargeBody("PED-001", "0", "http://localhost:8082/webhooks/payments"),
		"moeda diferente":  `{"order_id":"PED-001","amount":1,"currency":"USD","webhook_url":"http://localhost:8082/x"}`,
		"webhook relativo": chargeBody("PED-001", "59.80", "/webhooks/payments"),
		"webhook ftp":      chargeBody("PED-001", "59.80", "ftp://localhost/webhooks"),
	}

	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			recorder := newTestMock(t).do(http.MethodPost, "/charges", body, nil)

			if recorder.Code != http.StatusBadRequest ||
				!strings.Contains(recorder.Body.String(), `"code":"INVALID_REQUEST"`) {
				t.Fatalf("esperava 400 INVALID_REQUEST, recebeu %d %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestCheckoutPageShowsButtonsAndEscapesOrder(t *testing.T) {
	instance := newTestMock(t)
	charge := instance.createCharge(t, "<b>PED</b>", "http://localhost:8082/webhooks/payments")

	recorder := instance.do(http.MethodGet, "/checkout/"+charge.ChargeID, "", nil)
	page := recorder.Body.String()

	if recorder.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d", recorder.Code)
	}

	for _, expected := range []string{
		"Pagamento Aprovado",
		"Pagamento Recusado",
		`action="/checkout/` + charge.ChargeID + `/approve"`,
		`action="/checkout/` + charge.ChargeID + `/refuse"`,
		"R$ 59,80",
		"&lt;b&gt;PED&lt;/b&gt;",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("página sem %q:\n%s", expected, page)
		}
	}

	if strings.Contains(page, "<b>PED</b>") {
		t.Fatal("pedido não foi escapado")
	}
}

func TestApproveSendsSignedWebhook(t *testing.T) {
	receiver := newWebhookReceiver(t, http.StatusNoContent)
	instance := newTestMock(t)
	charge := instance.createCharge(t, "PED-001", receiver.server.URL)

	recorder := instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/approve", "", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("esperava 200, recebeu %d: %s", recorder.Code, recorder.Body.String())
	}

	if receiver.calls() != 1 {
		t.Fatalf("esperava 1 webhook, recebeu %d", receiver.calls())
	}

	body := receiver.bodies[0]
	if !webhook.Verify(testSecret, []byte(body), receiver.signatures[0]) {
		t.Fatal("webhook sem assinatura válida")
	}

	var payload WebhookPayload
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("webhook inválido: %v", err)
	}

	if !strings.HasPrefix(payload.EventID, "whk-") ||
		payload.ChargeID != charge.ChargeID ||
		payload.OrderID != "PED-001" ||
		payload.Status != StatusApproved ||
		!payload.OccurredAt.Equal(instance.now) {
		t.Fatalf("webhook inesperado: %+v", payload)
	}

	if !strings.Contains(recorder.Body.String(), "Pagamento aprovado") {
		t.Fatalf("página de resultado inesperada: %s", recorder.Body.String())
	}
}

func TestRefuseSendsRefusedWebhook(t *testing.T) {
	receiver := newWebhookReceiver(t, http.StatusNoContent)
	instance := newTestMock(t)
	charge := instance.createCharge(t, "PED-001", receiver.server.URL)

	instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/refuse", "", nil)

	if receiver.calls() != 1 || !strings.Contains(receiver.bodies[0], `"status":"REFUSED"`) {
		t.Fatalf("webhook de recusa inesperado: %v", receiver.bodies)
	}
}

func TestFailedWebhookCanBeRetriedWithSameEvent(t *testing.T) {
	receiver := newWebhookReceiver(t, http.StatusInternalServerError, http.StatusNoContent)
	instance := newTestMock(t)
	charge := instance.createCharge(t, "PED-001", receiver.server.URL)

	first := instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/approve", "", nil)
	if first.Code != http.StatusBadGateway {
		t.Fatalf("esperava 502 com falha no webhook, recebeu %d", first.Code)
	}

	second := instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/approve", "", nil)
	if second.Code != http.StatusOK {
		t.Fatalf("esperava 200 na nova tentativa, recebeu %d", second.Code)
	}

	if receiver.calls() != 2 || receiver.bodies[0] != receiver.bodies[1] {
		t.Fatalf("nova tentativa deve reenviar o mesmo evento: %v", receiver.bodies)
	}

	instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/approve", "", nil)
	if receiver.calls() != 2 {
		t.Fatal("webhook entregue foi reenviado")
	}
}

func TestConflictingDecisionIsRejected(t *testing.T) {
	receiver := newWebhookReceiver(t, http.StatusNoContent)
	instance := newTestMock(t)
	charge := instance.createCharge(t, "PED-001", receiver.server.URL)

	instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/approve", "", nil)
	recorder := instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/refuse", "", nil)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("esperava 409, recebeu %d", recorder.Code)
	}

	if receiver.calls() != 1 {
		t.Fatalf("decisão conflitante enviou webhook: %d", receiver.calls())
	}
}

func TestExpiredCheckoutRejectsDecision(t *testing.T) {
	receiver := newWebhookReceiver(t, http.StatusNoContent)
	instance := newTestMock(t)
	charge := instance.createCharge(t, "PED-001", receiver.server.URL)

	instance.now = instance.now.Add(31 * time.Minute)

	page := instance.do(http.MethodGet, "/checkout/"+charge.ChargeID, "", nil)
	if !strings.Contains(page.Body.String(), "Checkout expirado") ||
		strings.Contains(page.Body.String(), "/approve") {
		t.Fatalf("página expirada inesperada: %s", page.Body.String())
	}

	recorder := instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/approve", "", nil)
	if recorder.Code != http.StatusGone {
		t.Fatalf("esperava 410, recebeu %d", recorder.Code)
	}

	if receiver.calls() != 0 {
		t.Fatal("checkout expirado enviou webhook")
	}
}

func TestUnknownChargeReturnsNotFound(t *testing.T) {
	instance := newTestMock(t)

	for _, recorder := range []*httptest.ResponseRecorder{
		instance.do(http.MethodGet, "/checkout/inexistente", "", nil),
		instance.do(http.MethodPost, "/checkout/inexistente/approve", "", nil),
	} {
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("esperava 404, recebeu %d", recorder.Code)
		}
	}
}

func TestUnknownDecisionReturnsNotFound(t *testing.T) {
	instance := newTestMock(t)
	charge := instance.createCharge(t, "PED-001", "http://localhost:8082/webhooks/payments")

	recorder := instance.do(http.MethodPost, "/checkout/"+charge.ChargeID+"/talvez", "", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("esperava 404, recebeu %d", recorder.Code)
	}
}
