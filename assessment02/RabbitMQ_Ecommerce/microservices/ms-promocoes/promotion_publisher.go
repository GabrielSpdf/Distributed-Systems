package mspromocoes

import (
	"context"
	"crypto/rsa"
	"fmt"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

func PublishSignedPromotion(
	ctx context.Context,
	connection *amqp.Connection,
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

	if err := ctx.Err(); err != nil {
		return "", err
	}
	if connection == nil || connection.IsClosed() {
		return "", fmt.Errorf("conexão RabbitMQ indisponível")
	}

	envelope, err := BuildSignedPromotionEvent(payload, privateKey)
	if err != nil {
		return "", err
	}

	// Garante que a fila exista mesmo antes de iniciar o consumidor.
	channel, err := connection.Channel()
	if err != nil {
		return "", fmt.Errorf("erro ao abrir canal de preparação: %w", err)
	}
	err = rabbitmq.PreparePromotionOfferQueue(channel)
	closeErr := channel.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", fmt.Errorf("erro ao fechar canal de preparação: %w", closeErr)
	}

	return publishPromotionCopies(ctx, envelope, copies, func(ctx context.Context, envelope events.EventEnvelope) error {
		return rabbitmq.PublishConfirmed(ctx, connection, events.ExchangePromotions, envelope)
	})
}

// Cada cópia deve receber confirmação. Em caso de erro, o ID é retornado para
// diagnóstico: uma falha de confirmação não prova que o broker não recebeu.
func publishPromotionCopies(
	ctx context.Context,
	envelope events.EventEnvelope,
	copies int,
	publish func(context.Context, events.EventEnvelope) error,
) (string, error) {
	// O envelope é criado uma única vez: mesmo ID, conteúdo e assinatura.
	for i := 0; i < copies; i++ {
		if err := ctx.Err(); err != nil {
			return envelope.EventID, err
		}
		if err := publish(ctx, envelope); err != nil {
			return envelope.EventID, fmt.Errorf("oferta %s, cópia %d/%d não confirmada: %w", envelope.EventID, i+1, copies, err)
		}
	}

	return envelope.EventID, nil
}

// BuildSignedPromotionEvent é o único construtor usado pelos publicadores
// de ofertas, incluindo o comando de demonstração sem flags.
func BuildSignedPromotionEvent(
	payload events.PromotionPayload,
	privateKey *rsa.PrivateKey,
) (events.EventEnvelope, error) {
	if privateKey == nil {
		return events.EventEnvelope{}, fmt.Errorf("chave privada de Promoções não configurada")
	}
	category, err := normalizePromotionCategory(payload.Category)
	if err != nil {
		return events.EventEnvelope{}, err
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
		return events.EventEnvelope{}, err
	}

	envelope, err := misc.BuildSignedEnvelope(
		payload,
		routingKey,
		events.ProducerPromotions,
		privateKey,
	)
	if err != nil {
		return events.EventEnvelope{}, fmt.Errorf("erro ao assinar oferta: %w", err)
	}

	return envelope, nil
}
