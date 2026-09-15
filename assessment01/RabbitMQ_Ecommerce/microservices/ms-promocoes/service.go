package mspromocoes

import (
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"fmt"
	"log"
	"math/rand"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Runtime struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
}

func InitializePromotionService() (
	*Runtime,
	error,
) {
	connection, err := rabbitmq.Connect()
	if err != nil {
		return nil, err
	}
	log.Println("[SUCESSO] Conexão estabelecida com o RabbitMQ")

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Canal RabbitMQ aberto")

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Exchanges declaradas")

	runtime := &Runtime{
		Connection: connection,
		Channel:    channel,
	}

	return runtime, nil
}

func GeneratePromotion(products []events.Product) events.PromotionPayload {
	product := products[rand.Intn(len(products))]

	// rand.Float64() gera um numero entre 0 e 1 incluso
	discount := 5 + rand.Float64()*25 // (5% - 30%)
	promotionalPrice := product.Price * (1 - discount/100)

	return events.PromotionPayload{
		ProductID:          product.ID,
		ProductName:        product.Name,
		Category:           product.Category,
		DiscountPercentage: discount,
		OriginalPrice:      product.Price,
		PromotionalPrice:   promotionalPrice,
	}
}

// categoryRoutingKeys mapeia a categoria real do produto pra routing key
// exigida pelo enunciado
var categoryRoutingKeys = map[string]string{
	"Limpeza":     events.PromotionCategoryA,
	"Alimentos":   events.PromotionCategoryB,
	"Eletrônicos": events.PromotionCategoryC,
}

// RoutingKeyForCategory retorna a routing key correspondente a categoria do
// produto. O segundo valor eh false se a categoria não tiver mapeamento
func RoutingKeyForCategory(category string) (string, bool) {
	routingKey, exists := categoryRoutingKeys[category]
	return routingKey, exists
}

func PublishPromotion(
	channel *amqp.Channel,
	payload events.PromotionPayload,
	routingKey string,
) error {
	envelope, err := misc.BuildEnvelope(
		payload,
		routingKey,
		events.ProducerPromotions,
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangePromotions,
		routingKey,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}
