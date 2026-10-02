package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const sessionCookieName = "session"

type HTTPHandler struct {
	users  *UserRepository
	tokens *TokenManager
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func NewHTTPHandler(
	users *UserRepository,
	tokens *TokenManager,
) *HTTPHandler {
	return &HTTPHandler{
		users:  users,
		tokens: tokens,
	}
}

func (handler *HTTPHandler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(
		"POST /api/auth/register",
		handler.register,
	)
	mux.HandleFunc(
		"POST /api/auth/login",
		handler.login,
	)
	mux.HandleFunc(
		"GET /api/auth/me",
		handler.currentUser,
	)
	mux.HandleFunc(
		"POST /api/auth/logout",
		handler.logout,
	)

	return mux
}

func (handler *HTTPHandler) register(
	writer http.ResponseWriter,
	request *http.Request,
) {
	var input registerRequest

	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"JSON inválido",
		)
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = normalizeEmail(input.Email)

	if input.Name == "" {
		writeError(
			writer,
			http.StatusBadRequest,
			"nome é obrigatório",
		)
		return
	}

	if !validEmail(input.Email) {
		writeError(
			writer,
			http.StatusBadRequest,
			"e-mail inválido",
		)
		return
	}

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	user, err := handler.users.Create(
		request.Context(),
		input.Name,
		input.Email,
		passwordHash,
	)
	if errors.Is(err, ErrEmailAlreadyUsed) {
		writeError(
			writer,
			http.StatusConflict,
			"E-mail já cadastrado",
		)
		return
	}

	if err != nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"erro ao cadastrar usuário",
		)
		return
	}

	writeJSON(writer, http.StatusCreated, user)
}

func (handler *HTTPHandler) login(
	writer http.ResponseWriter,
	request *http.Request,
) {
	var input loginRequest

	if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"JSON inválido",
		)
		return
	}

	input.Email = normalizeEmail(input.Email)

	if input.Email == "" || input.Password == "" {
		writeError(
			writer,
			http.StatusBadRequest,
			"e-mail e senha são obrigatórios",
		)
		return
	}

	user, err := handler.users.FindByEmail(
		request.Context(),
		input.Email,
	)
	if errors.Is(err, ErrUserNotFound) {
		writeError(
			writer,
			http.StatusUnauthorized,
			"Credenciais inválidas",
		)
		return
	}

	if err != nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"erro ao realizar login",
		)
		return
	}

	if !CheckPassword(user.PasswordHash, input.Password) {
		writeError(
			writer,
			http.StatusUnauthorized,
			"Credenciais inválidas",
		)
		return
	}

	tokenValue, err := handler.tokens.Generate(user.ID)
	if err != nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"erro ao gerar sessão",
		)
		return
	}

	setSessionCookie(writer, tokenValue)
	writeJSON(writer, http.StatusOK, user)
}

func setSessionCookie(
	writer http.ResponseWriter,
	tokenValue string,
) {
	http.SetCookie(
		writer,
		&http.Cookie{
			Name:     sessionCookieName,
			Value:    tokenValue,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   int(tokenDuration.Seconds()),
		},
	)
}

func (handler *HTTPHandler) currentUser(
	writer http.ResponseWriter,
	request *http.Request,
) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		writeError(
			writer,
			http.StatusUnauthorized,
			"usuário não autenticado",
		)
		return
	}

	userID, err := handler.tokens.Validate(cookie.Value)
	if err != nil {
		writeError(
			writer,
			http.StatusUnauthorized,
			"sessão inválida ou expirada",
		)
		return
	}

	user, err := handler.users.FindByID(
		request.Context(),
		userID,
	)
	if errors.Is(err, ErrUserNotFound) {
		writeError(
			writer,
			http.StatusUnauthorized,
			"usuário não autenticado",
		)
		return
	}

	if err != nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"erro ao consultar usuário",
		)
		return
	}

	writeJSON(writer, http.StatusOK, user)
}

func (handler *HTTPHandler) logout(
	writer http.ResponseWriter,
	_ *http.Request,
) {
	http.SetCookie(
		writer,
		&http.Cookie{
			Name:     sessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0).UTC(),
		},
	)

	writer.WriteHeader(http.StatusNoContent)
}

func validEmail(email string) bool {
	address, err := mail.ParseAddress(email)

	return err == nil && address.Address == email
}

func writeError(
	writer http.ResponseWriter,
	status int,
	message string,
) {
	writeJSON(
		writer,
		status,
		errorResponse{
			Error: message,
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

	_ = json.NewEncoder(writer).Encode(value)
}
