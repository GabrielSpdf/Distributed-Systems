package mspromocoes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (repository *InterestRepository) ConsumeInterests(
	ctx context.Context,
	connection *amqp.Connection,
	publicKeys cryptography.PublicKeyRegistry,
) error {
	if connection == nil || connection.IsClosed() {
		return fmt.Errorf("conexão RabbitMQ indisponível")
	}
	if publicKeys[events.ProducerPrincipal] == nil {
		return fmt.Errorf("chave pública do Principal não configurada")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("erro ao abrir canal de interesses: %w", err)
	}
	defer channel.Close()

	if err := rabbitmq.PreparePromotionInterestQueue(channel); err != nil {
		return err
	}

	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("erro ao configurar consumo sequencial: %w", err)
	}

	deliveries, err := channel.Consume(
		events.QueuePromotionInterests,
		"",
		false, // confirmação manual
		false,
		true, // consumidor exclusivo: impede duas instâncias simultâneas
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("erro ao iniciar consumidor de interesses: %w", err)
	}

	log.Println("[SUCESSO] Consumidor de interesses conectado")

	for {
		select {
		case <-ctx.Done():
			return nil

		case delivery, open := <-deliveries:
			if !open {
				return fmt.Errorf("canal de entregas de interesses encerrado")
			}
			if ctx.Err() != nil {
				return nil
			}

			if len(delivery.Body) > 64*1024 {
				log.Println("[SEGURANÇA] Interesse rejeitado: mensagem muito grande")
				if err := delivery.Reject(false); err != nil {
					return fmt.Errorf("erro ao rejeitar mensagem: %w", err)
				}
				continue
			}

			var envelope events.EventEnvelope
			if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
				log.Println("[SEGURANÇA] Interesse rejeitado: JSON inválido")
				if err := delivery.Reject(false); err != nil {
					return fmt.Errorf("erro ao rejeitar mensagem: %w", err)
				}
				continue
			}

			attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			applied, applyErr := repository.ApplyInterestEvent(
				attemptCtx,
				envelope,
				delivery.RoutingKey,
				publicKeys,
			)
			cancel()

			if ctx.Err() != nil {
				return nil
			}

			if errors.Is(applyErr, ErrInvalidInterestEvent) {
				log.Printf("[SEGURANÇA] Interesse rejeitado: %v", applyErr)
				if err := delivery.Reject(false); err != nil {
					return fmt.Errorf("erro ao rejeitar evento inválido: %w", err)
				}
				continue
			}

			if applyErr != nil {
				// Recoloca a mensagem e encerra esta sessão.
				// O supervisor fará a reconexão com espera.
				if err := delivery.Nack(false, true); err != nil {
					return fmt.Errorf(
						"aplicação falhou: %v; erro ao reenfileirar: %w",
						applyErr, err,
					)
				}
				return fmt.Errorf("erro ao aplicar interesse: %w", applyErr)
			}

			// Evento aplicado ou duplicado já registrado: pode confirmar.
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("erro ao confirmar interesse: %w", err)
			}

			log.Printf(
				"[INFO] Interesse confirmado: evento=%s aplicado=%v",
				envelope.EventID,
				applied,
			)
		}
	}
}

func (repository *InterestRepository) RunInterestConsumer(
	ctx context.Context,
	rabbitURL string,
	publicKeys cryptography.PublicKeyRegistry,
) error {
	if publicKeys[events.ProducerPrincipal] == nil {
		return fmt.Errorf("chave pública do Principal não configurada")
	}

	for ctx.Err() == nil {
		connection, err := amqp.DialConfig(rabbitURL, amqp.Config{
			Heartbeat: 10 * time.Second,
			Dial:      amqp.DefaultDial(5 * time.Second),
		})

		if err == nil {
			stopClose := context.AfterFunc(ctx, func() {
				_ = connection.CloseDeadline(time.Now())
			})

			err = repository.ConsumeInterests(ctx, connection, publicKeys)

			stopClose()
			_ = connection.CloseDeadline(time.Now().Add(time.Second))
		}

		if ctx.Err() != nil {
			return nil
		}

		log.Printf("[ERRO] Consumidor de interesses desconectado: %v", err)

		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}

	return nil
}
