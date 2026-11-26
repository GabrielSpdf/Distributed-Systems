package orders

import (
	"encoding/json"
	"log"
	"net/http"
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
	mux.HandleFunc("GET /api/orders/events", handler.streamEvents)

	return mux
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
