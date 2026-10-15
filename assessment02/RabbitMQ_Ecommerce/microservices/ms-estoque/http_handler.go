package msestoque

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

const serviceName = "ms-estoque"

// CatalogStore is what the HTTP layer needs from the database.
type CatalogStore interface {
	ListProducts(ctx context.Context, filter ProductFilter) ([]Product, error)
	Ping(ctx context.Context) error
}

type HTTPHandler struct {
	store CatalogStore
}

func NewHTTPHandler(store CatalogStore) *HTTPHandler {
	return &HTTPHandler{
		store: store,
	}
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/products", handler.listProducts)
	mux.HandleFunc("GET /healthz", handler.health)

	return mux
}

func (handler *HTTPHandler) listProducts(
	writer http.ResponseWriter,
	request *http.Request,
) {
	query := request.URL.Query()

	filter := ProductFilter{
		Category: strings.ToLower(strings.TrimSpace(query.Get("category"))),
	}

	switch query.Get("available") {
	case "":
	case "true":
		available := true
		filter.Available = &available
	case "false":
		available := false
		filter.Available = &available
	default:
		writeJSON(writer, http.StatusBadRequest, errorResponse{
			Code:    "INVALID_REQUEST",
			Message: "available deve ser true ou false",
		})
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()

	products, err := handler.store.ListProducts(ctx, filter)
	if err != nil {
		log.Printf("[ERRO] Consulta de produtos: %v", err)

		writeJSON(writer, http.StatusInternalServerError, errorResponse{
			Code:    "INTERNAL_ERROR",
			Message: "não foi possível consultar os produtos",
		})
		return
	}

	if products == nil {
		products = []Product{}
	}

	writeJSON(writer, http.StatusOK, ProductsResponse{
		Products: products,
		Total:    len(products),
	})
}

func (handler *HTTPHandler) health(
	writer http.ResponseWriter,
	request *http.Request,
) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()

	if err := handler.store.Ping(ctx); err != nil {
		writeJSON(writer, http.StatusServiceUnavailable, healthResponse{
			Service: serviceName,
			Status:  "unhealthy",
		})
		return
	}

	writeJSON(writer, http.StatusOK, healthResponse{
		Service: serviceName,
		Status:  "healthy",
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("[ERRO] Escrita da resposta JSON: %v", err)
	}
}
