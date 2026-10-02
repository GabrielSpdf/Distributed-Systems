package auth

import (
	"errors"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	minPasswordLength = 8
	maxPasswordBytes  = 72
)

var (
	ErrPasswordTooShort = errors.New("a senha deve ter pelo menos 8 caracteres")
	ErrPasswordTooLong  = errors.New("a senha deve ter no máximo 72 bytes")
)

func HashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < minPasswordLength {
		return "", ErrPasswordTooShort
	}

	if len([]byte(password)) > maxPasswordBytes {
		return "", ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func CheckPassword(passwordHash string, password string) bool {
	return bcrypt.CompareHashAndPassword(
		[]byte(passwordHash),
		[]byte(password),
	) == nil
}
