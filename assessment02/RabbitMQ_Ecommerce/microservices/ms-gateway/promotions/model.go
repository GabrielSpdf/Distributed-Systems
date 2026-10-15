package promotions

import (
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"
)

// SubscribeRequest contém somente as categorias escolhidas pelo cliente.
type SubscribeRequest struct {
	Categories []string `json:"categories"`
}

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

func validateCategories(categories []string) error {
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
