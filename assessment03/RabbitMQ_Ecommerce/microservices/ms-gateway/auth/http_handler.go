package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const sessionCookieName = "session"
const maxNameCharacters = 120
const maxEmailBytes = 255

type UserStore interface {
	Create(context.Context, string, string, string) (User, error)
	FindByEmail(context.Context, string) (User, error)
	FindByID(context.Context, string) (User, error)
}

type HTTPHandler struct {
	users  UserStore
	tokens *TokenManager
}

func NewHTTPHandler(users UserStore, tokens *TokenManager) *HTTPHandler {
	return &HTTPHandler{users: users, tokens: tokens}
}

func (handler *HTTPHandler) Register(
	ctx context.Context,
	name, email, password string,
) (User, error) {
	name = strings.TrimSpace(name)
	email = normalizeEmail(email)

	if name == "" || utf8.RuneCountInString(name) > maxNameCharacters ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return User{}, errors.New("nome deve ter de 1 a 120 caracteres, sem caracteres de controle")
	}
	if !validEmail(email) {
		return User{}, errors.New("e-mail inválido")
	}

	passwordHash, err := HashPassword(password)
	if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrPasswordTooLong) {
		return User{}, err
	}
	if err != nil {
		return User{}, errors.New("erro ao proteger senha")
	}

	user, err := handler.users.Create(ctx, name, email, passwordHash)
	if errors.Is(err, ErrEmailAlreadyUsed) {
		return User{}, errors.New("E-mail já cadastrado")
	}
	if err != nil {
		return User{}, errors.New("erro ao cadastrar usuário")
	}
	return user, nil
}

func (handler *HTTPHandler) Login(
	ctx context.Context,
	writer http.ResponseWriter,
	email, password string,
) (User, error) {
	email = normalizeEmail(email)
	if email == "" || password == "" {
		return User{}, errors.New("e-mail e senha são obrigatórios")
	}
	if !validEmail(email) {
		return User{}, errors.New("e-mail inválido")
	}
	if len(password) > maxPasswordBytes {
		return User{}, ErrPasswordTooLong
	}

	user, err := handler.users.FindByEmail(ctx, email)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, errors.New("Credenciais inválidas")
	}
	if err != nil {
		return User{}, errors.New("erro ao realizar login")
	}
	if !CheckPassword(user.PasswordHash, password) {
		return User{}, errors.New("Credenciais inválidas")
	}

	token, err := handler.tokens.Generate(user.ID)
	if err != nil {
		return User{}, errors.New("erro ao gerar sessão")
	}

	cookie := sessionCookie()
	cookie.Value = token
	cookie.MaxAge = int(tokenDuration.Seconds())
	http.SetCookie(writer, cookie)
	return user, nil
}

func (handler *HTTPHandler) Logout(writer http.ResponseWriter) {
	cookie := sessionCookie()
	cookie.MaxAge = -1
	cookie.Expires = time.Unix(0, 0).UTC()
	http.SetCookie(writer, cookie)
}

func sessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func validEmail(email string) bool {
	if len(email) > maxEmailBytes || strings.IndexFunc(email, unicode.IsControl) >= 0 {
		return false
	}
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": message})
}
