package mspromocoes

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/mail"
	"strings"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (repository *EmailRepository) ConsumeOffers(
	ctx context.Context,
	connection *amqp.Connection,
	publicKeys cryptography.PublicKeyRegistry,
	from string,
	testRecipient string,
) error {
	if connection == nil || connection.IsClosed() {
		return fmt.Errorf("conexão RabbitMQ indisponível")
	}
	if publicKeys[events.ProducerPromotions] == nil {
		return fmt.Errorf("chave pública de Promoções não configurada")
	}

	from = strings.TrimSpace(from)
	testRecipient = strings.TrimSpace(testRecipient)

	if _, err := mail.ParseAddress(from); err != nil {
		return fmt.Errorf("remetente inválido")
	}
	address, err := mail.ParseAddress(testRecipient)
	if err != nil || address.Address != testRecipient {
		return fmt.Errorf("destinatário de teste inválido")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := rabbitmq.PreparePromotionOfferQueue(channel); err != nil {
		return err
	}
	if err := channel.Qos(1, 0, false); err != nil {
		return err
	}

	deliveries, err := channel.Consume(
		events.QueuePromotionOffers,
		"",
		false, // ACK manual, somente após gravar no banco.
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return err
	}

	log.Print("[INICIADO] Consumidor de ofertas conectado")

	for {
		select {
		case <-ctx.Done():
			return nil

		case delivery, open := <-deliveries:
			if !open {
				return fmt.Errorf("canal de ofertas encerrado")
			}
			if ctx.Err() != nil {
				return nil
			}

			var envelope events.EventEnvelope
			if len(delivery.Body) > 64*1024 ||
				json.Unmarshal(delivery.Body, &envelope) != nil {
				log.Print("[REJEITADO] Oferta com tamanho ou JSON inválido")
				if err := delivery.Reject(false); err != nil {
					return err
				}
				continue
			}

			payload, err := ValidatePromotionEvent(
				envelope, delivery.RoutingKey, publicKeys,
			)
			if err != nil {
				log.Printf("[REJEITADO] Oferta: %v", err)
				if err := delivery.Reject(false); err != nil {
					return err
				}
				continue
			}

			attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			created, queueErr := repository.QueuePromotionEmails(
				attemptCtx,
				envelope.EventID,
				payload,
				from,
				testRecipient,
			)
			cancel()

			if ctx.Err() != nil {
				return nil
			}
			if queueErr != nil {
				// Preserva a mensagem para tentar novamente.
				if err := delivery.Nack(false, true); err != nil {
					return fmt.Errorf(
						"gravação falhou: %v; reenfileiramento falhou: %w",
						queueErr, err,
					)
				}
				return fmt.Errorf("erro ao preparar envios: %w", queueErr)
			}

			if err := delivery.Ack(false); err != nil {
				return err
			}

			log.Printf(
				"[PROCESSADO] Oferta=%s categoria=%s novos envios=%d",
				envelope.EventID, payload.Category, created,
			)
		}
	}
}
