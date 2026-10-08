package orders

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

// ConsumeStatusEvents usa um canal exclusivo e o fecha ao terminar.
// Confirma a mensagem somente após armazená-la na inbox.
func (repository *Repository) ConsumeStatusEvents(
	ctx context.Context,
	channel *amqp.Channel,
	queueName string,
	publicKeys cryptography.PublicKeyRegistry,
) error {
	defer channel.Close()

	if err := channel.Qos(1, 0, false); err != nil {
		return fmt.Errorf("erro ao configurar consumo de status: %w", err)
	}

	deliveries, err := channel.Consume(
		queueName,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("erro ao iniciar consumo de status: %w", err)
	}

	reject := func(delivery amqp.Delivery, reason error) error {
		log.Printf("[SEGURANÇA] Evento de status rejeitado: %v", reason)
		if err := delivery.Nack(false, false); err != nil {
			return fmt.Errorf("erro ao rejeitar evento inválido: %w", err)
		}
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case delivery, open := <-deliveries:
			if !open {
				return errors.New("canal de entregas de status encerrado")
			}

			var envelope events.EventEnvelope
			if err := json.Unmarshal(delivery.Body, &envelope); err != nil {
				if err := reject(delivery, err); err != nil {
					return err
				}
				continue
			}

			if delivery.RoutingKey != envelope.EventType {
				err := errors.New("routing key diferente do tipo do evento")
				if err := reject(delivery, err); err != nil {
					return err
				}
				continue
			}

			if err := cryptography.VerifyEnvelopeFromProducer(
				envelope, publicKeys,
			); err != nil {
				if err := reject(delivery, err); err != nil {
					return err
				}
				continue
			}

			_, err := repository.StoreStatusEvent(ctx, envelope)
			if err != nil {
				if errors.Is(err, ErrInvalidStatusEvent) {
					if err := reject(delivery, err); err != nil {
						return err
					}
					continue
				}

				// Não confirma a entrega. O fechamento do canal
				// permite que o RabbitMQ a entregue novamente.
				return fmt.Errorf(
					"erro ao armazenar evento %s: %w",
					envelope.EventID,
					err,
				)
			}

			// Evento novo ou repetição idêntica já estão guardados.
			if err := delivery.Ack(false); err != nil {
				return fmt.Errorf("erro ao confirmar evento: %w", err)
			}
		}
	}
}

const StatusQueueName = "gateway.pedidos.status"

func PrepareStatusQueue(channel *amqp.Channel) error {
	if err := rabbitmq.DeclareExchange(
		channel,
		events.ExchangeEcommerce,
		rabbitmq.ExchangeTypeDirect,
	); err != nil {
		return err
	}

	queue, err := rabbitmq.DeclareQueue(channel, StatusQueueName)
	if err != nil {
		return err
	}

	for _, routingKey := range []string{
		events.OrderStockConfirmed,
		events.StockUnavailable,
		events.PaymentCheckoutAvailable,
		events.PaymentApproved,
		events.PaymentRefused,
		events.OrderShipped,
	} {
		if err := rabbitmq.BindQueue(
			channel,
			queue.Name,
			routingKey,
			events.ExchangeEcommerce,
		); err != nil {
			return err
		}
	}

	return nil
}

func waitStatusLoop(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (repository *Repository) RunStatusProcessor(ctx context.Context) {
	for ctx.Err() == nil {
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		handled, err := repository.ProcessNextStatusEvent(attemptCtx)
		cancel()

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			log.Printf("[ERRO] Processamento da inbox: %v", err)
			if !waitStatusLoop(ctx, 5*time.Second) {
				return
			}
			continue
		}

		if !handled && !waitStatusLoop(ctx, time.Second) {
			return
		}
	}
}

func (repository *Repository) consumeStatusConnection(
	ctx context.Context,
	rabbitURL string,
	publicKeys cryptography.PublicKeyRegistry,
) error {
	connection, err := rabbitmq.ConnectURL(rabbitURL)
	if err != nil {
		return err
	}
	defer connection.Close()

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := PrepareStatusQueue(channel); err != nil {
		return err
	}

	log.Println("[SUCESSO] Consumidor de status conectado")

	return repository.ConsumeStatusEvents(
		ctx, channel, StatusQueueName, publicKeys,
	)
}

func (repository *Repository) RunStatusConsumer(
	ctx context.Context,
	rabbitURL string,
	publicKeys cryptography.PublicKeyRegistry,
) {
	for ctx.Err() == nil {
		err := repository.consumeStatusConnection(ctx, rabbitURL, publicKeys)

		if ctx.Err() != nil {
			return
		}

		log.Printf(
			"[ERRO] Consumo de status encerrado: %v; nova tentativa em 5 segundos",
			err,
		)

		if !waitStatusLoop(ctx, 5*time.Second) {
			return
		}
	}
}
