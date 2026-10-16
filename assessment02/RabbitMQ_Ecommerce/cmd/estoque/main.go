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

	msestoque "RabbitMQ_Ecommerce/microservices/ms-estoque"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[ERRO] MS Estoque encerrado: %v", err)
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

	stockService, err := msestoque.InitializeStockService()
	if err != nil {
		return err
	}
	defer stockService.Connection.Close()
	defer stockService.Channel.Close()

	fmt.Println("[SUCESSO] Microsserviço estoque inicializado com sucesso")
	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO ESTOQUE                       ")
	fmt.Println("==================================================================")

	repository := msestoque.NewRepository(databasePool)

	eventHandler := msestoque.NewEventHandler(
		repository,
		msestoque.NewSigner(stockService.PrivateKey),
		msestoque.NewConfirmedPublisher(stockService.Connection),
	)

	pendingContext, cancelPending := context.WithCancel(context.Background())
	defer cancelPending()

	// Reenvia resultados gravados cuja publicação não foi confirmada.
	go eventHandler.RunPendingPublisher(pendingContext, 10*time.Second)

	server := &http.Server{
		Addr:              ":" + configuration.StockPort,
		Handler:           msestoque.NewHTTPHandler(repository).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("[SUCESSO] MS Estoque disponível em http://localhost%s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	consumerErrors := make(chan error, 1)
	go func() {
		consumerErrors <- rabbitmq.ConsumeSignedEventsReliably(
			stockService.Connection,
			stockService.Channel,
			stockService.QueueName,
			stockService.PublicKeys,
			eventHandler.Handle,
			rabbitmq.RetryOptions{},
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
