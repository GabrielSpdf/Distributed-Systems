package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	mockpagamento "RabbitMQ_Ecommerce/microservices/mock-pagamento"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/webhook"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("[ERRO] Mock de Pagamento encerrado: %v", err)
	}
}

func run() error {
	configuration := config.Load()

	if err := webhook.ValidateSecret(configuration.PaymentWebhookSecret); err != nil {
		return err
	}

	checkoutTTL, err := time.ParseDuration(configuration.MockCheckoutTTL)
	if err != nil || checkoutTTL <= 0 {
		return fmt.Errorf("MOCK_CHECKOUT_TTL inválido: %q", configuration.MockCheckoutTTL)
	}

	mock := mockpagamento.NewMock(mockpagamento.Options{
		PublicURL:     configuration.MockPaymentPublicURL,
		WebhookSecret: configuration.PaymentWebhookSecret,
		CheckoutTTL:   checkoutTTL,
	})

	server := &http.Server{
		Addr:              ":" + configuration.MockPaymentPort,
		Handler:           mock.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf(
		"[SUCESSO] Mock de Pagamento disponível em http://localhost%s (checkout válido por %s)",
		server.Addr,
		checkoutTTL,
	)

	return server.ListenAndServe()
}
