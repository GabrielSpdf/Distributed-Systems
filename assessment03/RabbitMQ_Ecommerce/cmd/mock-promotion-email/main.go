package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	mspromocoes "RabbitMQ_Ecommerce/microservices/ms-promocoes"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/events"
)

func main() {
	eventID := flag.String(
		"event-id", "", "Identificador único da oferta de teste",
	)
	flag.Parse()

	if err := run(*eventID); err != nil {
		log.Fatalf("[ERRO] %v", err)
	}
}

func run(eventID string) error {
	if eventID == "" {
		return fmt.Errorf("informe -event-id com um UUID")
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), 15*time.Second,
	)
	defer cancel()

	pool, err := database.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()

	repository := mspromocoes.NewEmailRepository(pool)

	created, err := repository.QueuePromotionEmails(
		ctx,
		eventID,
		events.PromotionPayload{
			ProductID:          "TEST-ALIMENTOS-001",
			ProductName:        "Arroz — oferta fictícia de teste",
			Category:           "alimentos",
			OriginalPrice:      10,
			PromotionalPrice:   9,
			DiscountPercentage: 10,
		},
		os.Getenv("RESEND_FROM"),
		os.Getenv("RESEND_TEST_RECIPIENT"),
	)
	if err != nil {
		return err
	}

	log.Printf(
		"[PREPARADO] Oferta=%s; novos envios pendentes=%d; nenhum e-mail enviado",
		eventID, created,
	)
	return nil
}
