package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	msgateway "RabbitMQ_Ecommerce/microservices/ms-gateway"
	"RabbitMQ_Ecommerce/utils/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[ERRO] API Gateway encerrado: %v", err)
	}
}

func run() error {
	configuration := config.Load()
	server := &http.Server{
		Addr:              ":" + configuration.GatewayPort,
		Handler:           msgateway.NewHandler(configuration.FrontendOrigin),
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
