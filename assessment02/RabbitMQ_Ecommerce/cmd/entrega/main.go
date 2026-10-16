package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	msentrega "RabbitMQ_Ecommerce/microservices/ms-entrega"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

const pendingInterval = 10 * time.Second

func main() {
	if err := run(); err != nil {
		log.Fatalf("[ERRO] MS Entrega encerrado: %v", err)
	}
}

func run() error {
	configuration := config.Load()

	databasePool, err := database.Connect(
		context.Background(),
		configuration.DatabaseURL,
	)
	if err != nil {
		return fmt.Errorf("erro ao inicializar o PostgreSQL: %w", err)
	}
	defer databasePool.Close()

	log.Println("[SUCESSO] Conexão com o PostgreSQL estabelecida")

	deliveryService, err := msentrega.InitializeDeliveryService()
	if err != nil {
		return err
	}
	defer deliveryService.Connection.Close()
	defer deliveryService.Channel.Close()

	fmt.Println("[SUCESSO] Microsserviço entrega inicializado com sucesso")
	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO ENTREGA                       ")
	fmt.Println("==================================================================")

	eventHandler := msentrega.NewEventHandler(
		msentrega.NewRepository(databasePool),
		msentrega.NewSigner(deliveryService.PrivateKey),
		msentrega.NewConfirmedPublisher(deliveryService.Connection),
	)

	pendingContext, cancelPending := context.WithCancel(context.Background())
	defer cancelPending()

	go eventHandler.RunPendingPublisher(pendingContext, pendingInterval)

	consumerErrors := make(chan error, 1)
	go func() {
		consumerErrors <- rabbitmq.ConsumeSignedEventsReliably(
			deliveryService.Connection,
			deliveryService.Channel,
			deliveryService.QueueName,
			deliveryService.PublicKeys,
			eventHandler.Handle,
			rabbitmq.RetryOptions{},
		)
	}()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-consumerErrors:
		if err != nil {
			return fmt.Errorf("consumidor de eventos encerrado: %w", err)
		}

		return errors.New("consumidor de eventos encerrado: canal RabbitMQ fechado")
	case <-interrupts:
		return nil
	}
}
