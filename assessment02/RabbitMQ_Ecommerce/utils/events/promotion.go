package events

// Define a estrutura do Payload do evento de promoção
type PromotionPayload struct {
	ProductID          string  `json:"product_id"`
	ProductName        string  `json:"product_name"`
	Category           string  `json:"category"`
	DiscountPercentage float64 `json:"discount_percentage"`
	OriginalPrice      float64 `json:"original_price"`
	PromotionalPrice   float64 `json:"promotional_price"`
}
