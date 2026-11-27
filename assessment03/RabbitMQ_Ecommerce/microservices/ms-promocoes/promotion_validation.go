package mspromocoes

import (
	"fmt"
	"math"
	"strings"

	"RabbitMQ_Ecommerce/utils/events"
)

func normalizePromotionCategory(category string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(category)) {
	case "alimentos":
		return "alimentos", nil
	case "limpeza":
		return "limpeza", nil
	case "eletronicos", "eletrônicos":
		return "eletronicos", nil
	default:
		return "", fmt.Errorf("categoria da promoção inválida")
	}
}

func validatePromotionPayload(promotion events.PromotionPayload) error {
	if _, err := normalizePromotionCategory(promotion.Category); err != nil {
		return err
	}

	productID := strings.TrimSpace(promotion.ProductID)
	productName := strings.TrimSpace(promotion.ProductName)

	if productID == "" || productName == "" ||
		len(productID) > 64 || len(productName) > 160 ||
		strings.ContainsAny(productName, "\r\n") {
		return fmt.Errorf("identificação do produto inválida")
	}

	for _, value := range []float64{
		promotion.OriginalPrice,
		promotion.PromotionalPrice,
		promotion.DiscountPercentage,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return fmt.Errorf("valor da promoção inválido")
		}
	}

	if promotion.OriginalPrice <= 0 ||
		promotion.PromotionalPrice < 0 ||
		promotion.PromotionalPrice > promotion.OriginalPrice ||
		promotion.DiscountPercentage <= 0 ||
		promotion.DiscountPercentage > 100 {
		return fmt.Errorf("preço ou desconto inválido")
	}

	expectedPrice := math.Round(
		promotion.OriginalPrice*
			(1-promotion.DiscountPercentage/100)*100,
	) / 100

	actualPrice := math.Round(promotion.PromotionalPrice*100) / 100
	if math.Abs(expectedPrice-actualPrice) > 0.01 {
		return fmt.Errorf("preço não corresponde ao desconto")
	}

	return nil
}
