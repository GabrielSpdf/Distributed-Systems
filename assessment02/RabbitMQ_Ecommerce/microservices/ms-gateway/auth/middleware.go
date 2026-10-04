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

func (handler *HTTPHandler) RequireAuth(
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(
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

		_, err = handler.users.FindByID(
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
				"erro ao verificar usuário",
			)
			return
		}

		ctx := context.WithValue(
			request.Context(),
			userIDKey,
			userID,
		)

		next.ServeHTTP(
			writer,
			request.WithContext(ctx),
		)
	})
}
