package promotions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
)

type HTTPHandler struct {
	repository *Repository
}

func NewHTTPHandler(repository *Repository) *HTTPHandler {
	return &HTTPHandler{repository: repository}
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/promotion-interests", handler.list)
	mux.HandleFunc("POST /api/promotion-interests", handler.subscribe)
	mux.HandleFunc(
		"DELETE /api/promotion-interests/{category}",
		handler.unsubscribe,
	)
	return mux
}

func (handler *HTTPHandler) list(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(request.Context())
	if !authenticated {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()

	categories, err := handler.repository.ListActive(ctx, userID)
	if err != nil {
		log.Printf("[ERRO] Consulta de preferências: %v", err)
		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível consultar as preferências",
		)
		return
	}

	if categories == nil {
		categories = []string{}
	}

	email, err := handler.repository.PreferredEmail(ctx, userID)
	if errors.Is(err, ErrUserNotFound) {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}
	if err != nil {
		log.Printf("[ERRO] Consulta do e-mail das promoções: %v", err)
		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível consultar as preferências",
		)
		return
	}

	writeJSON(writer, http.StatusOK, struct {
		Categories []string `json:"categories"`
		Email      string   `json:"email"`
	}{
		Categories: categories,
		Email:      email,
	})
}

func (handler *HTTPHandler) subscribe(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(request.Context())
	if !authenticated {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	request.Body = http.MaxBytesReader(writer, request.Body, 16*1024)

	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()

	var input SubscribeRequest
	if err := decoder.Decode(&input); err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"JSON inválido ou campos não permitidos",
		)
		return
	}

	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(writer, http.StatusBadRequest, "envie somente um objeto JSON")
		return
	}

	if err := validateCategories(input.Categories); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()

	changed, err := handler.repository.SubscribeWithEmail(
		ctx,
		userID,
		input.Categories,
		input.Email,
	)
	if errors.Is(err, ErrInvalidEmail) {
		writeError(writer, http.StatusBadRequest, "e-mail inválido")
		return
	}
	if errors.Is(err, ErrUserNotFound) {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}
	if err != nil {
		log.Printf("[ERRO] Inscrição em promoções: %v", err)
		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível salvar as preferências",
		)
		return
	}

	status := http.StatusOK
	if changed {
		status = http.StatusAccepted
	}

	writeJSON(writer, status, struct {
		Changed bool `json:"changed"`
	}{
		Changed: changed,
	})
}

func (handler *HTTPHandler) unsubscribe(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(request.Context())
	if !authenticated {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	category := request.PathValue("category")
	if err := validateCategories([]string{category}); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 5*time.Second)
	defer cancel()

	_, err := handler.repository.Unsubscribe(ctx, userID, category)
	if errors.Is(err, ErrUserNotFound) {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}
	if err != nil {
		log.Printf("[ERRO] Cancelamento de interesse: %v", err)
		writeError(
			writer,
			http.StatusInternalServerError,
			"não foi possível cancelar a preferência",
		)
		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func writeError(
	writer http.ResponseWriter,
	status int,
	message string,
) {
	writeJSON(writer, status, struct {
		Error string `json:"error"`
	}{
		Error: message,
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)

	if err := json.NewEncoder(writer).Encode(value); err != nil {
		log.Printf("[ERRO] Resposta de preferências: %v", err)
	}
}
