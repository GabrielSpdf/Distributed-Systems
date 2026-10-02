package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestHashPasswordAndCheckPassword(t *testing.T) {
	password := "senha-segura-123"

	passwordHash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("não esperava erro ao gerar hash: %v", err)
	}

	if passwordHash == password {
		t.Fatal("o hash não pode ser igual à senha original")
	}

	if !CheckPassword(passwordHash, password) {
		t.Fatal("a senha correta deveria ser aceita")
	}

	if CheckPassword(passwordHash, "senha-incorreta") {
		t.Fatal("uma senha incorreta não deveria ser aceita")
	}
}

func TestHashPasswordRejectsShortPassword(t *testing.T) {
	_, err := HashPassword("curta")

	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("esperava ErrPasswordTooShort, recebeu %v", err)
	}
}

func TestHashPasswordRejectsLongPassword(t *testing.T) {
	password := strings.Repeat("a", 73)

	_, err := HashPassword(password)

	if !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("esperava ErrPasswordTooLong, recebeu %v", err)
	}
}
