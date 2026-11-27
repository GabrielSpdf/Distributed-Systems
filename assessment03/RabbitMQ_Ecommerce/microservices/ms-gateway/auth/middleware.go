package auth

import (
	"errors"
	"net/http"
)

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
