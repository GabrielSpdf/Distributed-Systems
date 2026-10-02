package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestTokenManagerGenerateAndValidate(t *testing.T) {
	manager, err := NewTokenManager(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("não esperava erro ao criar gerenciador: %v", err)
	}

	tokenValue, err := manager.Generate("user-123")
	if err != nil {
		t.Fatalf("não esperava erro ao gerar token: %v", err)
	}

	userID, err := manager.Validate(tokenValue)
	if err != nil {
		t.Fatalf("não esperava erro ao validar token: %v", err)
	}

	if userID != "user-123" {
		t.Fatalf("esperava user-123, recebeu %s", userID)
	}
}

func TestTokenManagerRejectsShortSecret(t *testing.T) {
	_, err := NewTokenManager("segredo-curto")

	if !errors.Is(err, ErrJWTSecretTooShort) {
		t.Fatalf("esperava ErrJWTSecretTooShort, recebeu %v", err)
	}
}

func TestTokenManagerRejectsModifiedToken(t *testing.T) {
	manager, err := NewTokenManager(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("não esperava erro ao criar gerenciador: %v", err)
	}

	tokenValue, err := manager.Generate("user-123")
	if err != nil {
		t.Fatalf("não esperava erro ao gerar token: %v", err)
	}

	_, err = manager.Validate(tokenValue + "alterado")

	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("esperava ErrInvalidToken, recebeu %v", err)
	}
}

func TestTokenManagerRejectsExpiredToken(t *testing.T) {
	manager, err := NewTokenManager(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("não esperava erro ao criar gerenciador: %v", err)
	}

	expiredClaims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   "user-123",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}

	expiredToken := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		expiredClaims,
	)

	tokenValue, err := expiredToken.SignedString(manager.secret)
	if err != nil {
		t.Fatalf("não esperava erro ao assinar token: %v", err)
	}

	_, err = manager.Validate(tokenValue)

	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("esperava ErrInvalidToken, recebeu %v", err)
	}
}
