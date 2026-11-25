package mspagamento

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"regexp"
	"time"

	"RabbitMQ_Ecommerce/utils/webhook"
)

const (
	RefusedReason = "Pagamento recusado pelo usuário"

	serviceName       = "ms-pagamento"
	maxWebhookBytes   = 64 << 10
	webhookCtxTimeout = 10 * time.Second
)

var uuidPattern = regexp.MustCompile(
	`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`,
)

// ResultStore is what the webhook endpoint needs from the database.
type ResultStore interface {
	OutboxStore
	ApplyWebhook(ctx context.Context, event WebhookEvent, reason string) (WebhookOutcome, error)
	Ping(ctx context.Context) error
}

type WebhookHandler struct {
	store  ResultStore
	outbox *Outbox
	secret string
}

func NewWebhookHandler(
	store ResultStore,
	bus EventBus,
	secret string,
) *WebhookHandler {
	return &WebhookHandler{
		store:  store,
		outbox: NewOutbox(store, bus),
		secret: secret,
	}
}

func (handler *WebhookHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/payments", handler.receive)
	mux.HandleFunc("GET /healthz", handler.health)

	return mux
}

func (handler *WebhookHandler) receive(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, maxWebhookBytes))
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "corpo inválido")
		return
	}

	if !webhook.Verify(handler.secret, body, request.Header.Get(webhook.SignatureHeader)) {
		log.Println("[AVISO] Webhook com assinatura inválida recusado")
		writeError(writer, http.StatusUnauthorized, "INVALID_SIGNATURE", "assinatura inválida")
		return
	}

	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil || !validWebhook(event) {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "webhook inválido")
		return
	}

	reason := ""
	if event.Status == ChargeRefused {
		reason = RefusedReason
	}

	ctx, cancel := context.WithTimeout(request.Context(), webhookCtxTimeout)
	defer cancel()

	outcome, err := handler.store.ApplyWebhook(ctx, event, reason)
	switch {
	case errors.Is(err, ErrChargeNotFound):
		writeError(writer, http.StatusNotFound, "CHARGE_NOT_FOUND", "cobrança não encontrada")
		return
	case errors.Is(err, ErrInvalidWebhook):
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST", "cobrança não pertence ao pedido")
		return
	case errors.Is(err, ErrConflictingResult):
		writeError(writer, http.StatusConflict, "CONFLICTING_RESULT", "cobrança já possui outro resultado")
		return
	case errors.Is(err, ErrCheckoutExpired):
		writeError(writer, http.StatusConflict, "CHECKOUT_EXPIRED", "pagamento decidido depois do prazo do checkout")
		return
	case errors.Is(err, ErrWebhookEventReused):
		log.Printf("[AVISO] Webhook %s reutilizado para a cobrança %s", event.EventID, event.ChargeID)
		writeError(writer, http.StatusConflict, "EVENT_ID_CONFLICT", "event_id já usado em outra cobrança")
		return
	case err != nil:
		log.Printf("[ERRO] Webhook %s: %v", event.EventID, err)
		writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "não foi possível processar o webhook")
		return
	}

	charge := outcome.Charge

	if charge.ResultPublished {
		writer.WriteHeader(http.StatusNoContent)
		return
	}

	// Só uma cobrança decidida tem resultado para publicar.
	if charge.Status != ChargeApproved && charge.Status != ChargeRefused {
		log.Printf("[ERRO] Webhook %s sem resultado gravado para a cobrança %s", event.EventID, charge.ID)
		writeError(writer, http.StatusConflict, "CONFLICTING_RESULT", "cobrança sem resultado gravado")
		return
	}

	// Só falha se o evento não puder ser gravado: aí o Mock deve tentar de
	// novo. Falha no RabbitMQ deixa o evento pendente para a rotina de reenvio.
	if err := handler.outbox.PublishResult(ctx, charge); err != nil {
		log.Printf("[ERRO] Resultado da cobrança %s: %v", charge.ID, err)
		writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "não foi possível registrar o resultado")
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func validWebhook(event WebhookEvent) bool {
	return event.EventID != "" &&
		uuidPattern.MatchString(event.ChargeID) &&
		event.OrderID != "" &&
		(event.Status == ChargeApproved || event.Status == ChargeRefused) &&
		!event.OccurredAt.IsZero()
}

func (handler *WebhookHandler) health(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()

	if err := handler.store.Ping(ctx); err != nil {
		writeJSON(writer, http.StatusServiceUnavailable, map[string]string{
			"service": serviceName,
			"status":  "unhealthy",
		})
		return
	}

	writeJSON(writer, http.StatusOK, map[string]string{
		"service": serviceName,
		"status":  "healthy",
	})
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
