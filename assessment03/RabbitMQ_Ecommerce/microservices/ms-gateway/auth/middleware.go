package auth

import (
	"context"
	"errors"
	"net/http"
)

type contextKey string

const userIDKey contextKey = "authenticated_user_id"

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey).(string)

	return userID, ok && userID != ""
}

func (handler *HTTPHandler) CurrentUser(request *http.Request) (User, int, string) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		return User{}, http.StatusUnauthorized, "usuário não autenticado"
	}

	userID, err := handler.tokens.Validate(cookie.Value)
	if err != nil {
		return User{}, http.StatusUnauthorized, "sessão inválida ou expirada"
	}

	user, err := handler.users.FindByID(request.Context(), userID)
	if errors.Is(err, ErrUserNotFound) {
		return User{}, http.StatusUnauthorized, "usuário não autenticado"
	}
	if err != nil {
		return User{}, http.StatusInternalServerError, "erro ao verificar usuário"
	}
	return user, http.StatusOK, ""
}

func (handler *HTTPHandler) AuthenticateRequest(request *http.Request) (string, int, string) {
	user, status, message := handler.CurrentUser(request)
	return user.ID, status, message
}

func (handler *HTTPHandler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userID, status, message := handler.AuthenticateRequest(request)
		if status != http.StatusOK {
			writeError(writer, status, message)
			return
		}

		ctx := context.WithValue(request.Context(), userIDKey, userID)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}
