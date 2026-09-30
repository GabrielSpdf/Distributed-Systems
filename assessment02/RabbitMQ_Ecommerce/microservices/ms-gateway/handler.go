package msgateway

import (
	"encoding/json"
	"net/http"
)

type response struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

// NewHandler creates the public HTTP handler for the API Gateway.
func NewHandler(frontendOrigin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", serviceInformation)
	mux.HandleFunc("GET /healthz", health)

	return cors(frontendOrigin, mux)
}

func serviceInformation(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, response{
		Service: "ms-principal-api-gateway",
		Status:  "available",
	})
}

func health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, response{
		Service: "ms-principal-api-gateway",
		Status:  "healthy",
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	if err := json.NewEncoder(writer).Encode(value); err != nil {
		http.Error(writer, "erro ao gerar resposta", http.StatusInternalServerError)
	}
}

func cors(frontendOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Access-Control-Allow-Origin", frontendOrigin)
		writer.Header().Set("Access-Control-Allow-Credentials", "true")
		writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")

		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(writer, request)
	})
}
