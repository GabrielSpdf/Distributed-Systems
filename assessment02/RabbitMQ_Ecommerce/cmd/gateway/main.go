package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	msgateway "RabbitMQ_Ecommerce/microservices/ms-gateway"
	gatewayauth "RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
	gatewaycatalog "RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
	gatewayorders "RabbitMQ_Ecommerce/microservices/ms-gateway/orders"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[ERRO] API Gateway encerrado: %v", err)
	}
}

func run() error {
	configuration := config.Load()

	databasePool, err := database.Connect(
		context.Background(),
		configuration.DatabaseURL,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao inicializar o PostgreSQL: %w",
			err,
		)
	}
	defer databasePool.Close()

	rabbitConnection, err := rabbitmq.ConnectURL(
		configuration.RabbitMQURL,
	)
	if err != nil {
		return fmt.Errorf("erro ao inicializar RabbitMQ: %w", err)
	}
	defer rabbitConnection.Close()

	publisherChannel, err := rabbitmq.OpenChannel(rabbitConnection)
	if err != nil {
		return err
	}
	defer publisherChannel.Close()

	if err := rabbitmq.DeclareExchange(
		publisherChannel,
		events.ExchangeEcommerce,
		rabbitmq.ExchangeTypeDirect,
	); err != nil {
		return err
	}

	statusChannel, err := rabbitmq.OpenChannel(rabbitConnection)
	if err != nil {
		return err
	}
	defer statusChannel.Close()

	if err := gatewayorders.PrepareStatusQueue(statusChannel); err != nil {
		return fmt.Errorf("erro ao preparar fila de status: %w", err)
	}

	statusPublicKeys, err := cryptography.LoadPublicKeyRegistry(
		map[string]string{
			events.ProducerStock:    "keys/ms-estoque/public.pem",
			events.ProducerPayment:  "keys/ms-pagamento/public.pem",
			events.ProducerDelivery: "keys/ms-entrega/public.pem",
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao carregar chaves dos produtores: %w", err)
	}

	log.Printf(
		"[SUCESSO] Fila de status preparada; %d chaves públicas carregadas",
		len(statusPublicKeys),
	)

	log.Println("[SUCESSO] Conexão com o RabbitMQ estabelecida")

	privateKey, err := cryptography.LoadPrivateKey(
		"keys/ms-principal/private.pem",
	)
	if err != nil {
		return fmt.Errorf("erro ao carregar chave do Principal: %w", err)
	}

	orderPublisher := gatewayorders.NewEventPublisher(
		publisherChannel,
		privateKey,
	)

	tokenManager, err := gatewayauth.NewTokenManager(
		configuration.JWTSecret,
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao configurar autenticação: %w",
			err,
		)
	}

	userRepository := gatewayauth.NewUserRepository(
		databasePool,
	)

	authenticationHandler := gatewayauth.NewHTTPHandler(
		userRepository,
		tokenManager,
	)

	stockClient := gatewaycatalog.NewStockClient(
		configuration.StockServiceURL,
	)

	catalogHandler := gatewaycatalog.NewHTTPHandler(
		stockClient,
	)

	orderRepository := gatewayorders.NewRepository(
		databasePool,
	)

	statusContext, cancelStatus := context.WithCancel(context.Background())
	var statusWorkers sync.WaitGroup

	statusWorkers.Add(2)

	go func() {
		defer statusWorkers.Done()
		orderRepository.RunStatusConsumer(
			statusContext,
			configuration.RabbitMQURL,
			statusPublicKeys,
		)
	}()

	go func() {
		defer statusWorkers.Done()
		orderRepository.RunStatusProcessor(statusContext)
	}()

	defer func() {
		cancelStatus()
		statusWorkers.Wait()
	}()

	orderHandler := gatewayorders.NewHTTPHandler(
		orderRepository,
		stockClient,
		orderPublisher,
	)

	authenticatedOrderRoutes := authenticationHandler.RequireAuth(
		orderHandler.Routes(),
	)

	log.Println("[SUCESSO] Conexão com o PostgreSQL estabelecida")

	server := &http.Server{
		Addr:              ":" + configuration.GatewayPort,
		Handler:           msgateway.NewHandler(configuration.FrontendOrigin, databasePool, authenticationHandler.Routes(), catalogHandler.Routes(), authenticatedOrderRoutes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("[SUCESSO] API Gateway disponível em http://localhost%s", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("erro no servidor HTTP: %w", err)
	case <-interrupts:
		if err := server.Close(); err != nil {
			return fmt.Errorf("erro ao encerrar servidor HTTP: %w", err)
		}

		return nil
	}
}
