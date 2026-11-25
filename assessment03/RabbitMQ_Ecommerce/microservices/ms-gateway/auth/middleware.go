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

func (handler *HTTPHandler) AuthenticateRequest(
	request *http.Request,
) (userID string, status int, message string) {
	cookie, err := request.Cookie(sessionCookieName)
	if err != nil {
		return "", http.StatusUnauthorized, "usuário não autenticado"
	}

	userID, err = handler.tokens.Validate(cookie.Value)
	if err != nil {
		return "", http.StatusUnauthorized, "sessão inválida ou expirada"
	}

	_, err = handler.users.FindByID(request.Context(), userID)
	if errors.Is(err, ErrUserNotFound) {
		return "", http.StatusUnauthorized, "usuário não autenticado"
	}
	if err != nil {
		return "", http.StatusInternalServerError, "erro ao verificar usuário"
	}

	return userID, http.StatusOK, ""
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
