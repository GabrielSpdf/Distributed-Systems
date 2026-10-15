package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("GATEWAY_PORT", "")
	t.Setenv("FRONTEND_ORIGIN", "")
	t.Setenv("STOCK_SERVICE_URL", "")
	t.Setenv("STOCK_SERVICE_URL", "")
	t.Setenv("STOCK_PORT", "")
	t.Setenv("PAYMENT_PORT", "")
	t.Setenv("PAYMENT_WEBHOOK_URL", "")
	t.Setenv("PAYMENT_WEBHOOK_SECRET", "")
	t.Setenv("MOCK_PAYMENT_PORT", "")
	t.Setenv("MOCK_PAYMENT_URL", "")
	t.Setenv("MOCK_PAYMENT_PUBLIC_URL", "")
	t.Setenv("MOCK_CHECKOUT_TTL", "")

	configuration := Load()

	if configuration.RabbitMQURL != defaultRabbitMQURL {
		t.Fatalf("unexpected RabbitMQ URL: %s", configuration.RabbitMQURL)
	}

	if configuration.DatabaseURL != defaultDatabaseURL {
		t.Fatalf("unexpected database URL: %s", configuration.DatabaseURL)
	}

	if configuration.GatewayPort != defaultGatewayPort {
		t.Fatalf("unexpected gateway port: %s", configuration.GatewayPort)
	}

	if configuration.FrontendOrigin != defaultFrontendOrigin {
		t.Fatalf("unexpected frontend origin: %s", configuration.FrontendOrigin)
	}

	if configuration.StockServiceURL != defaultStockServiceURL {
		t.Fatalf(
			"unexpected stock service URL: %s",
			configuration.StockServiceURL,
		)
	}

	if configuration.StockServiceURL != defaultStockServiceURL {
		t.Fatalf(
			"unexpected stock service URL: %s",
			configuration.StockServiceURL,
		)
	}

	if configuration.StockPort != defaultStockPort {
		t.Fatalf("unexpected stock port: %s", configuration.StockPort)
	}

	if configuration.PaymentPort != defaultPaymentPort ||
		configuration.PaymentWebhookURL != defaultPaymentWebhookURL ||
		configuration.MockPaymentPort != defaultMockPaymentPort ||
		configuration.MockPaymentURL != defaultMockPaymentURL ||
		configuration.MockPaymentPublicURL != defaultMockPaymentPublicURL ||
		configuration.MockCheckoutTTL != defaultMockCheckoutTTL {
		t.Fatalf("unexpected payment defaults: %+v", configuration)
	}

	if configuration.PaymentWebhookSecret != "" {
		t.Fatal("webhook secret must not have a default")
	}
}

func TestLoadUsesEnvironmentValues(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqp://rabbitmq:5672/")
	t.Setenv("DATABASE_URL", "postgres://database/ecommerce")
	t.Setenv("GATEWAY_PORT", "9090")
	t.Setenv("FRONTEND_ORIGIN", "http://frontend:5173")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("STOCK_SERVICE_URL", "http://estoque:8081")
	t.Setenv("STOCK_PORT", "9091")
	t.Setenv("PAYMENT_PORT", "9092")
	t.Setenv("PAYMENT_WEBHOOK_URL", "http://pagamento:9092/webhooks/payments")
	t.Setenv("PAYMENT_WEBHOOK_SECRET", "webhook-secret")
	t.Setenv("MOCK_PAYMENT_PORT", "9093")
	t.Setenv("MOCK_PAYMENT_URL", "http://mock:9093")
	t.Setenv("MOCK_PAYMENT_PUBLIC_URL", "http://localhost:9093")
	t.Setenv("MOCK_CHECKOUT_TTL", "2m")

	configuration := Load()

	if configuration.RabbitMQURL != "amqp://rabbitmq:5672/" {
		t.Fatalf("unexpected RabbitMQ URL: %s", configuration.RabbitMQURL)
	}

	if configuration.DatabaseURL != "postgres://database/ecommerce" {
		t.Fatalf("unexpected database URL: %s", configuration.DatabaseURL)
	}

	if configuration.GatewayPort != "9090" {
		t.Fatalf("unexpected gateway port: %s", configuration.GatewayPort)
	}

	if configuration.FrontendOrigin != "http://frontend:5173" {
		t.Fatalf("unexpected frontend origin: %s", configuration.FrontendOrigin)
	}

	if configuration.JWTSecret != "test-secret" {
		t.Fatalf("unexpected JWT secret: %s", configuration.JWTSecret)
	}

	if configuration.StockServiceURL != "http://estoque:8081" {
		t.Fatalf(
			"unexpected stock service URL: %s",
			configuration.StockServiceURL,
		)
	}

	if configuration.StockPort != "9091" {
		t.Fatalf("unexpected stock port: %s", configuration.StockPort)
	}

	if configuration.PaymentPort != "9092" ||
		configuration.PaymentWebhookURL != "http://pagamento:9092/webhooks/payments" ||
		configuration.PaymentWebhookSecret != "webhook-secret" ||
		configuration.MockPaymentPort != "9093" ||
		configuration.MockPaymentURL != "http://mock:9093" ||
		configuration.MockPaymentPublicURL != "http://localhost:9093" ||
		configuration.MockCheckoutTTL != "2m" {
		t.Fatalf("unexpected payment values: %+v", configuration)
	}
}
