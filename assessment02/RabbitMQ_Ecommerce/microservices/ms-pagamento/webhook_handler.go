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

	"RabbitMQ_Ecommerce/utils/events"
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
	ApplyWebhook(ctx context.Context, event WebhookEvent, reason string) (WebhookOutcome, error)
	MarkResultPublished(ctx context.Context, chargeID string) error
	Ping(ctx context.Context) error
}

type WebhookHandler struct {
	store     ResultStore
	publisher EventPublisher
	secret    string
}

func NewWebhookHandler(
	store ResultStore,
	publisher EventPublisher,
	secret string,
) *WebhookHandler {
	return &WebhookHandler{
		store:     store,
		publisher: publisher,
		secret:    secret,
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

	if err := handler.publishResult(charge); err != nil {
		// Sem publicação o Gateway não seria avisado: o Mock deve tentar de novo.
		log.Printf("[ERRO] Resultado da cobrança %s: %v", charge.ID, err)
		writeError(writer, http.StatusInternalServerError, "PUBLISH_FAILED", "não foi possível publicar o resultado")
		return
	}

	if err := handler.store.MarkResultPublished(ctx, charge.ID); err != nil {
		log.Printf("[ERRO] Marcação do resultado da cobrança %s: %v", charge.ID, err)
	}

	if charge.Status == ChargeApproved {
		printBanner("PAGAMENTO APROVADO - PEDIDO - %s", charge.OrderID)
	} else {
		printBanner("PAGAMENTO RECUSADO - PEDIDO - %s", charge.OrderID)
	}
	log.Printf("[SUCESSO] Pagamento do pedido %s %s", charge.OrderID, charge.Status)

	writer.WriteHeader(http.StatusNoContent)
}

func validWebhook(event WebhookEvent) bool {
	return event.EventID != "" &&
		uuidPattern.MatchString(event.ChargeID) &&
		event.OrderID != "" &&
		(event.Status == ChargeApproved || event.Status == ChargeRefused) &&
		!event.OccurredAt.IsZero()
}

func (handler *WebhookHandler) publishResult(charge Charge) error {
	eventType := events.PaymentApproved
	reason := ""

	if charge.Status == ChargeRefused {
		eventType = events.PaymentRefused
		reason = charge.Reason
	}

	return handler.publisher.Publish(eventType, events.WebPaymentResultPayload{
		OrderID:     charge.OrderID,
		CustomerID:  charge.CustomerID,
		ChargeID:    charge.ID,
		Amount:      charge.Amount,
		Currency:    charge.Currency,
		Reason:      reason,
		ProcessedAt: charge.ProcessedAt,
	})
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
