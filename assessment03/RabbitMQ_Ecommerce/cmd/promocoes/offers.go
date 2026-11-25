package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	mspromocoes "RabbitMQ_Ecommerce/microservices/ms-promocoes"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

func runOfferMode() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	publicKeys, err := cryptography.LoadPublicKeyRegistry(
		map[string]string{
			events.ProducerPromotions: "keys/ms-promocoes/public.pem",
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao carregar chave de Promoções: %w", err)
	}

	configuration := config.Load()

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := database.Connect(startupCtx, configuration.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	defer pool.Close()

	connection, err := amqp.DialConfig(
		configuration.RabbitMQURL,
		amqp.Config{
			Heartbeat: 10 * time.Second,
			Dial:      amqp.DefaultDial(5 * time.Second),
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao conectar RabbitMQ: %w", err)
	}
	defer connection.Close()

	// Interrompe também a conexão quando receber Ctrl+C.
	stopClose := context.AfterFunc(ctx, func() {
		_ = connection.Close()
	})
	defer stopClose()

	repository := mspromocoes.NewEmailRepository(pool)

	err = repository.ConsumeOffers(
		ctx,
		connection,
		publicKeys,
		os.Getenv("RESEND_FROM"),
		os.Getenv("RESEND_TEST_RECIPIENT"),
	)
	if ctx.Err() != nil {
		return nil
	}
	return err
}
