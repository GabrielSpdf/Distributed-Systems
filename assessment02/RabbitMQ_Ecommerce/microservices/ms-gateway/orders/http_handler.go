package orders

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
)

type HTTPHandler struct {
	repository *Repository
	stock      *catalog.StockClient
}

func NewHTTPHandler(
	repository *Repository,
	stock *catalog.StockClient,
) *HTTPHandler {
	return &HTTPHandler{
		repository: repository,
		stock:      stock,
	}
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/orders", handler.create)
	mux.HandleFunc("GET /api/orders", handler.list)
	mux.HandleFunc("GET /api/orders/{orderId}", handler.details)

	return mux
}

func (handler *HTTPHandler) create(
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

	request.Body = http.MaxBytesReader(
		writer,
		request.Body,
		64*1024,
	)

	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	var input CreateOrderRequest

	if err := decoder.Decode(&input); err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"JSON inválido ou campos não permitidos",
		)
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(
			writer,
			http.StatusBadRequest,
			"envie somente um objeto JSON",
		)
		return
	}

	products, err := handler.stock.ListProducts(
		request.Context(),
		"",
		"",
	)
	if err != nil {
		log.Printf("[ERRO] Consulta ao estoque: %v", err)

		writeError(
			writer,
			http.StatusServiceUnavailable,
			"não foi possível consultar os produtos",
		)
		return
	}

	items, totalCents, err := PrepareItems(
		input.Items,
		products.Products,
	)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	order, err := handler.repository.Create(
		request.Context(),
		userID,
		items,
		totalCents,
	)
	if err != nil {
		log.Printf("[ERRO] Criação do pedido: %v", err)

		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível salvar o pedido",
		)
		return
	}

	addOrderLinks(&order)

	writeJSON(writer, http.StatusCreated, order)
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

func addOrderLinks(order *Order) {
	order.Links = map[string]Link{
		"self": {
			Href:   "/api/orders/" + order.ID,
			Method: http.MethodGet,
		},
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
