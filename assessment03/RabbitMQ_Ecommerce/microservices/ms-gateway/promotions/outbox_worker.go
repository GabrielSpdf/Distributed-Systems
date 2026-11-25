package promotions

import (
	"context"
	"crypto/rsa"
	"errors"
	"fmt"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func (repository *Repository) RunOutboxPublisher(
	ctx context.Context,
	rabbitURL string,
	privateKey *rsa.PrivateKey,
) {
	if privateKey == nil {
		log.Println("[ERRO] Publicador de interesses sem chave privada")
		return
	}

	publish := func(attemptCtx context.Context, data []byte) error {
		if err := attemptCtx.Err(); err != nil {
			return err
		}

		connection, err := amqp.DialConfig(rabbitURL, amqp.Config{
			Heartbeat: 10 * time.Second,
			Dial:      amqp.DefaultDial(5 * time.Second),
		})
		if err != nil {
			return fmt.Errorf("erro ao conectar publicador de interesses: %w", err)
		}

		stopClose := context.AfterFunc(attemptCtx, func() {
			_ = connection.CloseDeadline(time.Now())
		})
		defer func() {
			stopClose()
			_ = connection.CloseDeadline(time.Now().Add(time.Second))
		}()

		return PublishConfirmedInterest(attemptCtx, connection, data)
	}

	log.Println("[SUCESSO] Publicador da outbox de promoções iniciado")

	for ctx.Err() == nil {
		stepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		handled, err := repository.processNextOutbox(
			stepCtx, privateKey, publish,
		)
		cancel()

		if ctx.Err() != nil {
			return
		}

		delay := time.Second
		if err != nil {
			log.Printf("[ERRO] Outbox de promoções: %v", err)
			delay = 5 * time.Second
		} else if handled {
			log.Println("[INFO] Interesse publicado e registrado na outbox")
			continue
		}

		if !waitPromotionLoop(ctx, delay) {
			return
		}
	}
}

func (repository *Repository) processNextOutbox(
	ctx context.Context,
	privateKey *rsa.PrivateKey,
	publish func(context.Context, []byte) error,
) (bool, error) {
	eventID, err := repository.NextPendingOutboxEventID(ctx)
	if err != nil || eventID == "" {
		return false, err
	}

	err = repository.PrepareOutboxEnvelope(ctx, eventID, privateKey)
	if errors.Is(err, ErrOutboxEventNotFound) {
		// Outro processo pode ter finalizado o evento.
		return false, nil
	}
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}

		// Reagenda falhas de preparação para não monopolizar o processamento.
		_, recordErr := repository.pool.Exec(ctx, `
			UPDATE gateway.promotion_outbox
			SET attempt_count = attempt_count + 1,
			    status = CASE
			        WHEN attempt_count + 1 >= 12
			        THEN 'PENDENTE_REVISAO'
			        ELSE 'PENDENTE'
			    END,
			    next_attempt_at = clock_timestamp() + INTERVAL '5 seconds',
			    last_error = $2
			WHERE event_id = $1
			  AND status = 'PENDENTE'
			  AND envelope IS NULL
		`, eventID, err.Error())
		if recordErr != nil {
			return false, fmt.Errorf(
				"preparação falhou: %v; registro da falha: %w",
				err, recordErr,
			)
		}

		return false, fmt.Errorf("erro ao preparar interesse: %w", err)
	}

	return repository.PublishPreparedOutboxEvent(ctx, eventID, publish)
}

func waitPromotionLoop(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
