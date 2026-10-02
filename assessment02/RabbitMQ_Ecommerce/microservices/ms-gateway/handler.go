package msgateway

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type DatabasePinger interface {
	Ping(ctx context.Context) error
}

type response struct {
	Service string `json:"service"`
	Status  string `json:"status"`
}

// NewHandler creates the public HTTP handler for the API Gateway.
func NewHandler(frontendOrigin string, database DatabasePinger, authenticationRoutes http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", serviceInformation)
	mux.HandleFunc(
		"GET /healthz",
		func(writer http.ResponseWriter, request *http.Request) {
			health(writer, request, database)
		},
	)
	mux.Handle(
		"/api/auth/",
		authenticationRoutes,
	)

	return cors(frontendOrigin, mux)
}

func serviceInformation(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, response{
		Service: "ms-principal-api-gateway",
		Status:  "available",
	})
}

func health(writer http.ResponseWriter, _ *http.Request, database DatabasePinger) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		2*time.Second,
	)
	defer cancel()

	if err := database.Ping(ctx); err != nil {
		writeJSON(
			writer,
			http.StatusServiceUnavailable,
			response{
				Service: "ms-principal-api-gateway",
				Status:  "unhealthy",
			},
		)

		return
	}

	writeJSON(
		writer,
		http.StatusOK,
		response{
			Service: "ms-principal-api-gateway",
			Status:  "healthy",
		},
	)
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
