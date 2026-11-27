package msestoque

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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

type healthResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("POST /graphql", NewGraphQLHandler(handler.store))
	mux.HandleFunc("GET /healthz", handler.health)

	return mux
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
