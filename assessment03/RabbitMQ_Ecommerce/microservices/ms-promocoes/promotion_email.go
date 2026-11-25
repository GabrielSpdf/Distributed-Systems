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

func BuildPromotionEmail(
	promotion events.PromotionPayload,
	recipient string,
) (EmailMessage, error) {
	if _, err := normalizePromotionCategory(promotion.Category); err != nil {
		return EmailMessage{}, err
	}

	productID := strings.TrimSpace(promotion.ProductID)
	productName := strings.TrimSpace(promotion.ProductName)

	if productID == "" || productName == "" ||
		len(productID) > 64 || len(productName) > 160 ||
		strings.ContainsAny(productName, "\r\n") {
		return EmailMessage{}, fmt.Errorf("identificação do produto inválida")
	}

	for _, value := range []float64{
		promotion.OriginalPrice,
		promotion.PromotionalPrice,
		promotion.DiscountPercentage,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return EmailMessage{}, fmt.Errorf("valor da promoção inválido")
		}
	}

	if promotion.OriginalPrice <= 0 ||
		promotion.PromotionalPrice < 0 ||
		promotion.PromotionalPrice > promotion.OriginalPrice ||
		promotion.DiscountPercentage <= 0 ||
		promotion.DiscountPercentage > 100 {
		return EmailMessage{}, fmt.Errorf("preço ou desconto inválido")
	}

	expectedPrice := math.Round(
		promotion.OriginalPrice*
			(1-promotion.DiscountPercentage/100)*100,
	) / 100

	actualPrice := math.Round(promotion.PromotionalPrice*100) / 100
	if math.Abs(expectedPrice-actualPrice) > 0.01 {
		return EmailMessage{}, fmt.Errorf("preço não corresponde ao desconto")
	}

	formatPrice := func(value float64) string {
		return "R$ " + strings.ReplaceAll(fmt.Sprintf("%.2f", value), ".", ",")
	}

	text := fmt.Sprintf(
		"Olá!\n\nConfira esta oferta do Event Market:\n\n"+
			"Produto: %s\nPreço original: %s\nPreço promocional: %s\n\n"+
			"Você pode cancelar seu interesse nessa categoria "+
			"na seção Preferências de promoções do Event Market.",
		productName,
		formatPrice(promotion.OriginalPrice),
		formatPrice(promotion.PromotionalPrice),
	)

	return EmailMessage{
		To:      recipient,
		Subject: "Oferta Event Market: " + productName,
		Text:    text,
	}, nil
}
