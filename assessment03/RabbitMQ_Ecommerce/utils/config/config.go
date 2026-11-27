package config

import "os"

const (
	defaultRabbitMQURL          = "amqp://guest:guest@localhost:5672/"
	defaultDatabaseURL          = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce?sslmode=disable"
	defaultGatewayPort          = "8080"
	defaultFrontendOrigin       = "http://localhost:5173"
	defaultStockServiceURL      = "http://localhost:8081"
	defaultStockPort            = "8081"
	defaultPaymentPort          = "8082"
	defaultPaymentWebhookURL    = "http://localhost:8082/webhooks/payments"
	defaultMockPaymentPort      = "8083"
	defaultMockPaymentURL       = "http://localhost:8083"
	defaultMockPaymentPublicURL = "http://localhost:8083"
	defaultMockCheckoutTTL      = "30m"
)

// Environment centralizes configuration shared by the applications.
type Environment struct {
	RabbitMQURL          string
	DatabaseURL          string
	StockServiceURL      string
	StockPort            string
	GatewayPort          string
	FrontendOrigin       string
	JWTSecret            string
	PaymentPort          string
	PaymentWebhookURL    string
	PaymentWebhookSecret string
	MockPaymentPort      string
	MockPaymentURL       string
	MockPaymentPublicURL string
	MockCheckoutTTL      string
}

// Load reads application configuration from environment variables.
func Load() Environment {
	return Environment{
		RabbitMQURL:          valueOrDefault("RABBITMQ_URL", defaultRabbitMQURL),
		DatabaseURL:          valueOrDefault("DATABASE_URL", defaultDatabaseURL),
		StockServiceURL:      valueOrDefault("STOCK_SERVICE_URL", defaultStockServiceURL),
		StockPort:            valueOrDefault("STOCK_PORT", defaultStockPort),
		GatewayPort:          valueOrDefault("GATEWAY_PORT", defaultGatewayPort),
		FrontendOrigin:       valueOrDefault("FRONTEND_ORIGIN", defaultFrontendOrigin),
		JWTSecret:            os.Getenv("JWT_SECRET"),
		PaymentPort:          valueOrDefault("PAYMENT_PORT", defaultPaymentPort),
		PaymentWebhookURL:    valueOrDefault("PAYMENT_WEBHOOK_URL", defaultPaymentWebhookURL),
		PaymentWebhookSecret: os.Getenv("PAYMENT_WEBHOOK_SECRET"),
		MockPaymentPort:      valueOrDefault("MOCK_PAYMENT_PORT", defaultMockPaymentPort),
		MockPaymentURL:       valueOrDefault("MOCK_PAYMENT_URL", defaultMockPaymentURL),
		MockPaymentPublicURL: valueOrDefault("MOCK_PAYMENT_PUBLIC_URL", defaultMockPaymentPublicURL),
		MockCheckoutTTL:      valueOrDefault("MOCK_CHECKOUT_TTL", defaultMockCheckoutTTL),
	}
}

func valueOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
