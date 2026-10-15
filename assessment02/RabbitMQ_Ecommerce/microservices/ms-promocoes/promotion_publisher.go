package mspromocoes

import (
	"crypto/rsa"
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishSignedPromotion(
	channel *amqp.Channel,
	payload events.PromotionPayload,
	privateKey *rsa.PrivateKey,
	repetitions ...int,
) (string, error) {
	copies := 1
	if len(repetitions) > 1 {
		return "", fmt.Errorf("informe somente uma quantidade de repetições")
	}
	if len(repetitions) == 1 {
		copies = repetitions[0]
	}
	if copies < 1 || copies > 10 {
		return "", fmt.Errorf("repetições devem estar entre 1 e 10")
	}

	if channel == nil {
		return "", fmt.Errorf("canal RabbitMQ não informado")
	}

	category, err := normalizePromotionCategory(payload.Category)
	if err != nil {
		return "", err
	}

	routingKey := map[string]string{
		"limpeza":     events.PromotionCategoryA,
		"alimentos":   events.PromotionCategoryB,
		"eletronicos": events.PromotionCategoryC,
	}[category]

	// Valida os dados sem enviar e-mail.
	if _, err := BuildPromotionEmail(
		payload, "validacao@example.com",
	); err != nil {
		return "", err
	}

	envelope, err := misc.BuildSignedEnvelope(
		payload,
		routingKey,
		events.ProducerPromotions,
		privateKey,
	)
	if err != nil {
		return "", fmt.Errorf("erro ao assinar oferta: %w", err)
	}

	// Garante que a fila exista mesmo antes de iniciar o consumidor.
	if err := rabbitmq.PreparePromotionOfferQueue(channel); err != nil {
		return "", err
	}

	// O envelope é criado uma única vez: mesmo ID, conteúdo e assinatura.
	for i := 0; i < copies; i++ {
		if err := rabbitmq.PublishEvent(
			channel,
			events.ExchangePromotions,
			routingKey,
			envelope,
		); err != nil {
			return envelope.EventID, err
		}
	}

	return envelope.EventID, nil
}
