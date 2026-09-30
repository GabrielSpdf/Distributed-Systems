package config

import "os"

const (
	defaultRabbitMQURL    = "amqp://guest:guest@localhost:5672/"
	defaultDatabaseURL    = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce?sslmode=disable"
	defaultGatewayPort    = "8080"
	defaultFrontendOrigin = "http://localhost:5173"
)

// Environment centralizes configuration shared by the applications.
type Environment struct {
	RabbitMQURL    string
	DatabaseURL    string
	GatewayPort    string
	FrontendOrigin string
	JWTSecret      string
}

// Load reads application configuration from environment variables.
func Load() Environment {
	return Environment{
		RabbitMQURL:    valueOrDefault("RABBITMQ_URL", defaultRabbitMQURL),
		DatabaseURL:    valueOrDefault("DATABASE_URL", defaultDatabaseURL),
		GatewayPort:    valueOrDefault("GATEWAY_PORT", defaultGatewayPort),
		FrontendOrigin: valueOrDefault("FRONTEND_ORIGIN", defaultFrontendOrigin),
		JWTSecret:      os.Getenv("JWT_SECRET"),
	}
}

func valueOrDefault(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}
