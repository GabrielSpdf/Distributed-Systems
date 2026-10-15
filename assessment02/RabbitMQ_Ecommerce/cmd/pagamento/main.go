package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mspagamento "RabbitMQ_Ecommerce/microservices/ms-pagamento"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"RabbitMQ_Ecommerce/utils/webhook"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[ERRO] MS Pagamento encerrado: %v", err)
	}
}

func run() error {
	configuration := config.Load()

	if err := webhook.ValidateSecret(configuration.PaymentWebhookSecret); err != nil {
		return err
	}

	databasePool, err := database.Connect(
		context.Background(),
		configuration.DatabaseURL,
	)
	if err != nil {
		return fmt.Errorf("erro ao inicializar o PostgreSQL: %w", err)
	}
	defer databasePool.Close()

	log.Println("[SUCESSO] Conexão com o PostgreSQL estabelecida")

	paymentService, err := mspagamento.InitializePaymentService()
	if err != nil {
		return err
	}
	defer paymentService.Connection.Close()
	defer paymentService.Channel.Close()

	fmt.Println("[SUCESSO] Microsserviço pagamento inicializado com sucesso")
	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO PAGAMENTO                     ")
	fmt.Println("==================================================================")

	repository := mspagamento.NewRepository(databasePool)
	publisher := mspagamento.NewSignedPublisher(
		paymentService.Channel,
		paymentService.PrivateKey,
	)

	eventHandler := mspagamento.NewEventHandler(
		repository,
		mspagamento.NewMockClient(mspagamento.MockClientOptions{
			BaseURL:    configuration.MockPaymentURL,
			WebhookURL: configuration.PaymentWebhookURL,
		}),
		publisher,
	)

	server := &http.Server{
		Addr: ":" + configuration.PaymentPort,
		Handler: mspagamento.NewWebhookHandler(
			repository,
			publisher,
			configuration.PaymentWebhookSecret,
		).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("[SUCESSO] MS Pagamento disponível em http://localhost%s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	consumerErrors := make(chan error, 1)
	go func() {
		consumerErrors <- rabbitmq.ConsumeSignedEvents(
			paymentService.Channel,
			paymentService.QueueName,
			paymentService.PublicKeys,
			eventHandler.Handle,
		)
	}()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("erro no servidor HTTP: %w", err)
	case err := <-consumerErrors:
		if err != nil {
			return fmt.Errorf("consumidor de eventos encerrado: %w", err)
		}

		return errors.New("consumidor de eventos encerrado: canal RabbitMQ fechado")
	case <-interrupts:
		if err := server.Close(); err != nil {
			return fmt.Errorf("erro ao encerrar servidor HTTP: %w", err)
		}

		return nil
	}
}
