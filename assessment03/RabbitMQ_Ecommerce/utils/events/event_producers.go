package events

var expectedProducerByEventType = map[string]string{
	OrderCreated:             ProducerPrincipal,
	OrderDeleted:             ProducerPrincipal,
	OrderStockConfirmed:      ProducerStock,
	StockUnavailable:         ProducerStock,
	PaymentApproved:          ProducerPayment,
	PaymentRefused:           ProducerPayment,
	OrderShipped:             ProducerDelivery,
	PaymentCheckoutAvailable: ProducerPayment,
	PromotionInterest:        ProducerPrincipal,
	PromotionCategoryA:       ProducerPromotions,
	PromotionCategoryB:       ProducerPromotions,
	PromotionCategoryC:       ProducerPromotions,
}

// ExpectedProducerFor returns the producer authorized to publish an event type.
func ExpectedProducerFor(eventType string) (string, bool) {
	producer, exists := expectedProducerByEventType[eventType]
	return producer, exists
}
