package catalog

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

type HTTPHandler struct {
	stock *StockClient
}

func NewHTTPHandler(stock *StockClient) *HTTPHandler {
	return &HTTPHandler{
		stock: stock,
	}
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /api/products",
		handler.listProducts,
	)

	return mux
}

func (handler *HTTPHandler) listProducts(
	writer http.ResponseWriter,
	request *http.Request,
) {
	category := strings.TrimSpace(
		request.URL.Query().Get("category"),
	)
	available := request.URL.Query().Get("available")

	if available != "" &&
		available != "true" &&
		available != "false" {
		writeJSON(
			writer,
			http.StatusBadRequest,
			map[string]string{
				"error": "available deve ser true ou false",
			},
		)
		return
	}

	result, err := handler.stock.ListProducts(
		request.Context(),
		category,
		available,
	)
	if err != nil {
		log.Printf("[ERRO] Consulta ao estoque: %v", err)

		writeJSON(
			writer,
			http.StatusServiceUnavailable,
			map[string]string{
				"error": "Catálogo temporariamente indisponível",
			},
		)
		return
	}

	writeJSON(writer, http.StatusOK, result)
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
