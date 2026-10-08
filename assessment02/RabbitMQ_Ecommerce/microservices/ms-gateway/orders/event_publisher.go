package orders

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

type EventPublisher struct {
	channel    *amqp.Channel
	privateKey *rsa.PrivateKey
	mutex      sync.Mutex
}

func NewEventPublisher(
	channel *amqp.Channel,
	privateKey *rsa.PrivateKey,
) *EventPublisher {
	return &EventPublisher{
		channel:    channel,
		privateKey: privateKey,
	}
}

func (publisher *EventPublisher) PublishCreated(order Order) error {
	if publisher.channel == nil {
		return fmt.Errorf("canal RabbitMQ não configurado")
	}

	event, err := BuildOrderCreatedEvent(order, publisher.privateKey)
	if err != nil {
		return fmt.Errorf("erro ao preparar pedido.criado: %w", err)
	}

	publisher.mutex.Lock()
	defer publisher.mutex.Unlock()

	return rabbitmq.PublishEvent(
		publisher.channel,
		events.ExchangeEcommerce,
		events.OrderCreated,
		event,
	)
}

// Publica um envelope já assinado em um canal exclusivo desta tentativa.
func PublishConfirmedEvent(
	ctx context.Context,
	connection *amqp.Connection,
	envelope events.EventEnvelope,
) error {
	if connection == nil || connection.IsClosed() {
		return fmt.Errorf("conexão RabbitMQ indisponível")
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("erro ao serializar envelope: %w", err)
	}

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("erro ao abrir canal de publicação: %w", err)
	}
	defer channel.Close()

	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("erro ao ativar confirmações: %w", err)
	}

	returns := channel.NotifyReturn(make(chan amqp.Return, 1))

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(
		ctx,
		events.ExchangeEcommerce,
		envelope.EventType,
		true, // mandatory: devolve mensagens sem destino
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    envelope.EventID,
			Body:         data,
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao publicar compensação: %w", err)
	}
	if confirmation == nil {
		return fmt.Errorf("confirmação da publicação não disponível")
	}

	ack, err := confirmation.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("erro ao aguardar confirmação: %w", err)
	}
	if !ack {
		return fmt.Errorf("publicação não confirmada pelo RabbitMQ")
	}

	// Uma mensagem sem destino pode receber ACK e também ser devolvida.
	// A biblioteca entrega basic.return antes da confirmação correspondente.
	select {
	case returned, open := <-returns:
		if !open {
			return fmt.Errorf("canal fechado durante a publicação")
		}
		return fmt.Errorf(
			"mensagem devolvida pelo RabbitMQ: %d %s",
			returned.ReplyCode,
			returned.ReplyText,
		)
	default:
		return nil
	}
}

func (repository *Repository) RunOutboxPublisher(
	ctx context.Context,
	rabbitURL string,
	privateKey *rsa.PrivateKey,
) {
	if privateKey == nil {
		log.Println("[ERRO] Publicador da outbox sem chave privada")
		return
	}

	publish := func(
		attemptCtx context.Context,
		envelope events.EventEnvelope,
	) error {
		if err := attemptCtx.Err(); err != nil {
			return err
		}

		// Conexão própria: uma falha não reutiliza o canal antigo.
		connection, err := amqp.DialConfig(
			rabbitURL,
			amqp.Config{
				Heartbeat: 10 * time.Second,
				Dial:      amqp.DefaultDial(5 * time.Second),
			},
		)
		if err != nil {
			return fmt.Errorf("erro ao conectar publicador da outbox: %w", err)
		}

		// Interrompe operações da conexão quando a tentativa expirar.
		stopClose := context.AfterFunc(attemptCtx, func() {
			_ = connection.CloseDeadline(time.Now())
		})
		defer func() {
			stopClose()
			_ = connection.CloseDeadline(time.Now().Add(time.Second))
		}()

		return PublishConfirmedEvent(attemptCtx, connection, envelope)
	}

	log.Println("[SUCESSO] Publicador da outbox iniciado")

	for ctx.Err() == nil {
		handled, err := repository.ProcessNextOutboxEvent(
			ctx, privateKey, publish,
		)

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			log.Printf("[ERRO] Processamento da outbox: %v", err)
			if !waitStatusLoop(ctx, 5*time.Second) {
				return
			}
			continue
		}

		if handled {
			log.Println("[INFO] Compensação publicada e registrada pela outbox")
			continue
		}

		if !waitStatusLoop(ctx, time.Second) {
			return
		}
	}
}
