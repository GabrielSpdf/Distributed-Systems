package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	minJWTSecretBytes = 32
	tokenDuration     = 24 * time.Hour
	tokenIssuer       = "rabbitmq-ecommerce"
)

var (
	ErrJWTSecretTooShort = errors.New("JWT_SECRET deve ter pelo menos 32 bytes")
	ErrInvalidToken      = errors.New("token inválido ou expirado")
)

type Claims struct {
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret []byte
}

func NewTokenManager(secret string) (*TokenManager, error) {
	if len([]byte(secret)) < minJWTSecretBytes {
		return nil, ErrJWTSecretTooShort
	}

	return &TokenManager{
		secret: []byte(secret),
	}, nil
}

func (manager *TokenManager) Generate(userID string) (string, error) {
	now := time.Now().UTC()

	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(tokenDuration)),
		},
	}

	token := jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	)

	return token.SignedString(manager.secret)
}

func (manager *TokenManager) Validate(tokenValue string) (string, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(
		tokenValue,
		claims,
		func(_ *jwt.Token) (any, error) {
			return manager.secret, nil
		},
		jwt.WithValidMethods(
			[]string{jwt.SigningMethodHS256.Alg()},
		),
		jwt.WithIssuer(tokenIssuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid || claims.Subject == "" {
		return "", ErrInvalidToken
	}

	return claims.Subject, nil
}
