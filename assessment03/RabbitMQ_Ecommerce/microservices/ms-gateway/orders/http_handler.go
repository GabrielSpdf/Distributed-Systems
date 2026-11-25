package orders

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
)

type HTTPHandler struct {
	repository *Repository
}

func NewHTTPHandler(
	repository *Repository,
) *HTTPHandler {
	return &HTTPHandler{
		repository: repository,
	}
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/orders", handler.list)
	mux.HandleFunc("GET /api/orders/{orderId}", handler.details)
	mux.HandleFunc("DELETE /api/orders/{orderId}", handler.cancel)
	mux.HandleFunc("GET /api/orders/events", handler.streamEvents)

	return mux
}

func (handler *HTTPHandler) list(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(
		request.Context(),
	)
	if !authenticated {
		writeError(
			writer,
			http.StatusUnauthorized,
			"usuário não autenticado",
		)
		return
	}

	orders, err := handler.repository.ListByUser(
		request.Context(),
		userID,
	)
	if err != nil {
		log.Printf("[ERRO] Listagem de pedidos: %v", err)

		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível consultar os pedidos",
		)
		return
	}

	for index := range orders {
		addOrderLinks(&orders[index])
	}

	writeJSON(
		writer,
		http.StatusOK,
		struct {
			Orders []Order `json:"orders"`
			Total  int     `json:"total"`
		}{
			Orders: orders,
			Total:  len(orders),
		},
	)
}

func (handler *HTTPHandler) details(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(
		request.Context(),
	)
	if !authenticated {
		writeError(
			writer,
			http.StatusUnauthorized,
			"usuário não autenticado",
		)
		return
	}

	order, err := handler.repository.FindByID(
		request.Context(),
		userID,
		request.PathValue("orderId"),
	)
	if errors.Is(err, ErrOrderNotFound) {
		writeError(
			writer,
			http.StatusNotFound,
			"pedido não encontrado",
		)
		return
	}

	if err != nil {
		log.Printf("[ERRO] Consulta do pedido: %v", err)

		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível consultar o pedido",
		)
		return
	}

	addOrderLinks(&order)
	writeJSON(writer, http.StatusOK, order)
}

func (handler *HTTPHandler) cancel(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(request.Context())
	if !authenticated {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	_, err := handler.repository.Cancel(
		request.Context(),
		userID,
		request.PathValue("orderId"),
	)

	if errors.Is(err, ErrOrderNotFound) {
		writeError(writer, http.StatusNotFound, "pedido não encontrado")
		return
	}
	if errors.Is(err, ErrCancellationConflict) {
		writeError(
			writer,
			http.StatusConflict,
			"o processamento já começou; não é mais possível cancelar este pedido",
		)
		return
	}
	if err != nil {
		log.Printf("[ERRO] Cancelamento do pedido: %v", err)
		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível cancelar o pedido",
		)
		return
	}

	// A compensação foi salva na outbox; a publicação é assíncrona.
	writer.WriteHeader(http.StatusNoContent)
}

func addOrderLinks(order *Order) {
	order.Links = map[string]Link{
		"self": {
			Href:   "/api/orders/" + order.ID,
			Method: http.MethodGet,
		},
	}

	if order.Status == StatusPending && order.CanCancel {
		order.Links["cancel"] = Link{
			Href:   "/api/orders/" + order.ID,
			Method: http.MethodDelete,
		}
	}

	if order.Status == StatusAwaitingPayment &&
		order.CheckoutExpiresAt != nil &&
		order.CheckoutExpiresAt.After(time.Now()) {

		checkoutURL, err := url.Parse(order.CheckoutURL)
		if err == nil &&
			(checkoutURL.Scheme == "http" || checkoutURL.Scheme == "https") &&
			checkoutURL.Hostname() != "" &&
			checkoutURL.User == nil {

			order.Links["checkout"] = Link{
				Href:   order.CheckoutURL,
				Method: http.MethodGet,
			}
		}
	}
}

func writeError(
	writer http.ResponseWriter,
	status int,
	message string,
) {
	writeJSON(
		writer,
		status,
		map[string]string{
			"error": message,
		},
	)
}

func writeJSON(
	writer http.ResponseWriter,
	status int,
	value any,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("[ERRO] Escrita da resposta JSON: %v", err)
	}
}
