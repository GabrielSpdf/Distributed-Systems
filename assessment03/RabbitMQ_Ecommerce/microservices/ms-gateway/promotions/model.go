package promotions

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"RabbitMQ_Ecommerce/utils/events"
)

const (
	ActionSubscribe   = events.PromotionSubscribe
	ActionUnsubscribe = events.PromotionUnsubscribe
)

// Alias do contrato compartilhado, preservando os usos existentes no Gateway.
type InterestPayload = events.PromotionInterestPayload

var allowedCategories = map[string]struct{}{
	"alimentos":   {},
	"limpeza":     {},
	"eletronicos": {},
}

func ValidateCategories(categories []string) error {
	if len(categories) == 0 {
		return fmt.Errorf("selecione pelo menos uma categoria")
	}

	if len(categories) > len(allowedCategories) {
		return fmt.Errorf("selecione no máximo três categorias")
	}

	seen := make(map[string]struct{}, len(categories))

	for _, category := range categories {
		if _, allowed := allowedCategories[category]; !allowed {
			return fmt.Errorf("categoria inválida: %q", category)
		}

		if _, duplicate := seen[category]; duplicate {
			return fmt.Errorf("categoria repetida: %q", category)
		}

		seen[category] = struct{}{}
	}

	return nil
}

var ErrInvalidEmail = errors.New("e-mail inválido")

// normalizeEmail aceita um único endereço simples (sem nome de exibição) e o
// devolve em minúsculas.
func normalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))

	if email == "" || len(email) > 254 {
		return "", ErrInvalidEmail
	}

	address, err := mail.ParseAddress(email)
	if err != nil || address.Name != "" || address.Address != email {
		return "", ErrInvalidEmail
	}

	return email, nil
}
