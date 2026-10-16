// Package mockpagamento simula um provedor de pagamentos externo: cria
// cobranças, mostra a página de checkout e avisa o MS Pagamento por webhook.
package mockpagamento

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"RabbitMQ_Ecommerce/utils/webhook"
)

const (
	StatusPending  = "PENDING"
	StatusApproved = "APPROVED"
	StatusRefused  = "REFUSED"

	maxRequestBytes = 64 << 10
	webhookTimeout  = 5 * time.Second
)

// Options configura o Mock. Now e HTTPClient podem ser trocados nos testes.
type Options struct {
	PublicURL     string
	WebhookSecret string
	CheckoutTTL   time.Duration
	Now           func() time.Time
	HTTPClient    *http.Client
}

type ChargeRequest struct {
	OrderID    string  `json:"order_id"`
	Amount     float64 `json:"amount"`
	Currency   string  `json:"currency"`
	WebhookURL string  `json:"webhook_url"`
}

type ChargeResponse struct {
	ChargeID    string    `json:"charge_id"`
	OrderID     string    `json:"order_id"`
	Status      string    `json:"status"`
	CheckoutURL string    `json:"checkout_url"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// WebhookPayload é o corpo enviado ao MS Pagamento (contratos, seção 7).
type WebhookPayload struct {
	EventID    string    `json:"event_id"`
	ChargeID   string    `json:"charge_id"`
	OrderID    string    `json:"order_id"`
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurred_at"`
}

type charge struct {
	request          ChargeRequest
	response         ChargeResponse
	webhookEventID   string
	decidedAt        time.Time
	webhookDelivered bool
	// rejectedByStore: o MS Pagamento recusou a decisão (prazo vencido).
	rejectedByStore bool
}

// errRejectedByStore: o MS Pagamento respondeu 409 (decisão fora do prazo).
var errRejectedByStore = errors.New("MS Pagamento recusou a decisão")

type Mock struct {
	options Options

	mu            sync.Mutex
	charges       map[string]*charge
	chargesByKeys map[string]*charge
}

func NewMock(options Options) *Mock {
	if options.Now == nil {
		options.Now = time.Now
	}

	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: webhookTimeout}
	}

	options.PublicURL = strings.TrimRight(options.PublicURL, "/")

	return &Mock{
		options:       options,
		charges:       make(map[string]*charge),
		chargesByKeys: make(map[string]*charge),
	}
}

func (mock *Mock) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charges", mock.createCharge)
	mux.HandleFunc("GET /checkout/{chargeID}", mock.checkoutPage)
	mux.HandleFunc("POST /checkout/{chargeID}/{decision}", mock.decide)
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusOK, map[string]string{
			"service": "mock-pagamento",
			"status":  "healthy",
		})
	})

	return mux
}

func (mock *Mock) createCharge(writer http.ResponseWriter, request *http.Request) {
	var body ChargeRequest

	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxRequestBytes))
	if err := decoder.Decode(&body); err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "JSON inválido")
		return
	}

	if err := validateChargeRequest(body); err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" {
		key = body.OrderID
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if existing, found := mock.chargesByKeys[key]; found {
		if existing.request != body {
			writeError(
				writer,
				http.StatusConflict,
				"IDEMPOTENCY_CONFLICT",
				"Idempotency-Key já usada com outra cobrança",
			)
			return
		}

		writeJSON(writer, http.StatusOK, existing.response)
		return
	}

	chargeID, err := newUUID()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "não foi possível criar a cobrança")
		return
	}

	created := &charge{
		request: body,
		response: ChargeResponse{
			ChargeID:    chargeID,
			OrderID:     body.OrderID,
			Status:      StatusPending,
			CheckoutURL: mock.options.PublicURL + "/checkout/" + chargeID,
			ExpiresAt:   mock.options.Now().UTC().Add(mock.options.CheckoutTTL),
		},
	}

	mock.charges[chargeID] = created
	mock.chargesByKeys[key] = created

	log.Printf("[SUCESSO] Cobrança %s criada para o pedido %s", chargeID, body.OrderID)

	writeJSON(writer, http.StatusCreated, created.response)
}

func validateChargeRequest(body ChargeRequest) error {
	if strings.TrimSpace(body.OrderID) == "" {
		return errors.New("order_id é obrigatório")
	}

	if body.Amount <= 0 || math.IsNaN(body.Amount) || math.IsInf(body.Amount, 0) {
		return errors.New("amount deve ser positivo")
	}

	if body.Currency != "BRL" {
		return errors.New("currency deve ser BRL")
	}

	webhookURL, err := url.Parse(body.WebhookURL)
	if err != nil ||
		(webhookURL.Scheme != "http" && webhookURL.Scheme != "https") ||
		webhookURL.Host == "" {
		return errors.New("webhook_url deve ser uma URL HTTP absoluta")
	}

	return nil
}

func (mock *Mock) checkoutPage(writer http.ResponseWriter, request *http.Request) {
	mock.mu.Lock()
	found, exists := mock.charges[request.PathValue("chargeID")]
	var view pageView
	if exists {
		view = mock.viewOf(found, "")
	}
	mock.mu.Unlock()

	if !exists {
		renderPage(writer, http.StatusNotFound, pageView{Message: "Cobrança não encontrada."})
		return
	}

	renderPage(writer, http.StatusOK, view)
}

func (mock *Mock) decide(writer http.ResponseWriter, request *http.Request) {
	var requested string

	switch request.PathValue("decision") {
	case "approve":
		requested = StatusApproved
	case "refuse":
		requested = StatusRefused
	default:
		http.NotFound(writer, request)
		return
	}

	mock.mu.Lock()

	found, exists := mock.charges[request.PathValue("chargeID")]
	if !exists {
		mock.mu.Unlock()
		renderPage(writer, http.StatusNotFound, pageView{Message: "Cobrança não encontrada."})
		return
	}

	now := mock.options.Now().UTC()

	switch {
	case found.response.Status == StatusPending:
		if !now.Before(found.response.ExpiresAt) {
			view := mock.viewOf(found, "")
			mock.mu.Unlock()
			renderPage(writer, http.StatusGone, view)
			return
		}

		eventID, err := newUUID()
		if err != nil {
			mock.mu.Unlock()
			renderPage(writer, http.StatusInternalServerError, pageView{Message: "Falha interna."})
			return
		}

		found.response.Status = requested
		found.webhookEventID = "whk-" + eventID
		found.decidedAt = now

	case found.response.Status != requested:
		view := mock.viewOf(found, "Esta cobrança já foi decidida.")
		mock.mu.Unlock()
		renderPage(writer, http.StatusConflict, view)
		return

	case found.webhookDelivered:
		view := mock.viewOf(found, "")
		status := http.StatusOK
		if found.rejectedByStore {
			status = http.StatusConflict
		}
		mock.mu.Unlock()
		renderPage(writer, status, view)
		return
	}

	payload := WebhookPayload{
		EventID:    found.webhookEventID,
		ChargeID:   found.response.ChargeID,
		OrderID:    found.response.OrderID,
		Status:     found.response.Status,
		OccurredAt: found.decidedAt,
	}
	webhookURL := found.request.WebhookURL
	mock.mu.Unlock()

	err := mock.sendWebhook(request.Context(), webhookURL, payload)

	if errors.Is(err, errRejectedByStore) {
		// A loja (MS Pagamento) é a autoridade do prazo: a recusa é definitiva.
		log.Printf("[AVISO] Webhook %s recusado pela loja: checkout expirado", payload.EventID)

		mock.mu.Lock()
		found.webhookDelivered = true
		found.rejectedByStore = true
		view := mock.viewOf(found, "")
		mock.mu.Unlock()

		renderPage(writer, http.StatusConflict, view)
		return
	}

	if err != nil {
		log.Printf("[ERRO] Webhook da cobrança %s: %v", payload.ChargeID, err)

		mock.mu.Lock()
		view := mock.viewOf(found, "Não foi possível avisar a loja. Tente novamente.")
		mock.mu.Unlock()

		renderPage(writer, http.StatusBadGateway, view)
		return
	}

	mock.mu.Lock()
	found.webhookDelivered = true
	view := mock.viewOf(found, "")
	mock.mu.Unlock()

	log.Printf("[SUCESSO] Webhook %s enviado: cobrança %s %s", payload.EventID, payload.ChargeID, payload.Status)

	renderPage(writer, http.StatusOK, view)
}

func (mock *Mock) sendWebhook(
	ctx context.Context,
	webhookURL string,
	payload WebhookPayload,
) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("erro ao serializar webhook: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, webhookTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("erro ao montar webhook: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(webhook.SignatureHeader, webhook.Sign(mock.options.WebhookSecret, body))

	response, err := mock.options.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("erro ao enviar webhook: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusConflict {
		return errRejectedByStore
	}

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("MS Pagamento respondeu HTTP %d", response.StatusCode)
	}

	return nil
}

// viewOf monta os dados da página. Deve ser chamado com mock.mu travado.
func (mock *Mock) viewOf(found *charge, message string) pageView {
	expired := found.response.Status == StatusPending &&
		!mock.options.Now().UTC().Before(found.response.ExpiresAt)

	return pageView{
		Found:     true,
		ChargeID:  found.response.ChargeID,
		OrderID:   found.response.OrderID,
		Amount:    formatBRL(found.request.Amount),
		ExpiresAt: found.response.ExpiresAt.Local().Format("02/01/2006 15:04"),
		Status:    found.response.Status,
		Expired:   expired,
		Pending:   found.response.Status == StatusPending && !expired,
		Rejected:  found.rejectedByStore,
		Message:   message,
	}
}

func formatBRL(amount float64) string {
	return "R$ " + strings.Replace(fmt.Sprintf("%.2f", amount), ".", ",", 1)
}

func newUUID() (string, error) {
	var bytes [16]byte

	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("erro ao gerar identificador: %w", err)
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%x-%x-%x-%x-%x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("[ERRO] Escrita da resposta JSON: %v", err)
	}
}

func writeError(writer http.ResponseWriter, status int, code string, message string) {
	writeJSON(writer, status, map[string]string{
		"code":    code,
		"message": message,
	})
}
