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

	msgateway "RabbitMQ_Ecommerce/microservices/ms-gateway"
	gatewayauth "RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
	gatewaycatalog "RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
	gatewayorders "RabbitMQ_Ecommerce/microservices/ms-gateway/orders"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/database"
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

	orderHandler := gatewayorders.NewHTTPHandler(
		orderRepository,
		stockClient,
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
