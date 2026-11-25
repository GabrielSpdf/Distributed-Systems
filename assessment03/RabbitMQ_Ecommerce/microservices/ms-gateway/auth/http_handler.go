package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const sessionCookieName = "session"
const maxAuthBodyBytes = 16 * 1024
const maxNameCharacters = 120
const maxEmailBytes = 255

// UserStore permite testar a validação HTTP sem consultar o banco.
type UserStore interface {
	Create(context.Context, string, string, string) (User, error)
	FindByEmail(context.Context, string) (User, error)
	FindByID(context.Context, string) (User, error)
}

type HTTPHandler struct {
	users  UserStore
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
	users UserStore,
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

	if !decodeAuthJSON(writer, request, &input) {
		return
	}

	input.Name = strings.TrimSpace(input.Name)
	input.Email = normalizeEmail(input.Email)

	if input.Name == "" || utf8.RuneCountInString(input.Name) > maxNameCharacters ||
		strings.IndexFunc(input.Name, unicode.IsControl) >= 0 {
		writeError(
			writer,
			http.StatusBadRequest,
			"nome deve ter de 1 a 120 caracteres, sem caracteres de controle",
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
		if !errors.Is(err, ErrPasswordTooShort) && !errors.Is(err, ErrPasswordTooLong) {
			writeError(writer, http.StatusInternalServerError, "erro ao proteger senha")
			return
		}
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

	if !decodeAuthJSON(writer, request, &input) {
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

	if !validEmail(input.Email) {
		writeError(writer, http.StatusBadRequest, "e-mail inválido")
		return
	}
	if len(input.Password) > maxPasswordBytes {
		writeError(writer, http.StatusBadRequest, ErrPasswordTooLong.Error())
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
	if len(email) > maxEmailBytes || strings.IndexFunc(email, unicode.IsControl) >= 0 {
		return false
	}
	address, err := mail.ParseAddress(email)

	return err == nil && address.Address == email
}

// Limita também espaços e conteúdo extra após o primeiro objeto.
func decodeAuthJSON(writer http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxAuthBodyBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		err = decoder.Decode(&struct{}{})
		if err == io.EOF {
			return true
		}
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(writer, http.StatusRequestEntityTooLarge, "corpo da requisição excede 16 KiB")
	} else {
		writeError(writer, http.StatusBadRequest, "envie somente um objeto JSON válido, sem campos extras")
	}
	return false
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
