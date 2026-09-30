package config

import "testing"

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("GATEWAY_PORT", "")
	t.Setenv("FRONTEND_ORIGIN", "")

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
}

func TestLoadUsesEnvironmentValues(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqp://rabbitmq:5672/")
	t.Setenv("DATABASE_URL", "postgres://database/ecommerce")
	t.Setenv("GATEWAY_PORT", "9090")
	t.Setenv("FRONTEND_ORIGIN", "http://frontend:5173")
	t.Setenv("JWT_SECRET", "test-secret")

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
}
