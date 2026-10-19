package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mspromocoes "RabbitMQ_Ecommerce/microservices/ms-promocoes"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/database"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/inventory"
)

func main() {
	interests := flag.Bool(
		"interests", false, "Consumir interesses de promoções",
	)
	testEmail := flag.Bool(
		"test-email", false, "Enviar um único e-mail de teste",
	)
	emails := flag.Bool(
		"emails", false, "Processar e-mails pendentes",
	)
	offers := flag.Bool(
		"offers", false, "Consumir ofertas e preparar e-mails",
	)
	flag.Parse()

	modes := 0
	for _, enabled := range []bool{*interests, *testEmail, *emails, *offers} {
		if enabled {
			modes++
		}
	}

	var err error

	switch {
	case modes > 1:
		err = fmt.Errorf("use somente um modo por execução")
	case *offers:
		err = runOfferMode()
	case *emails:
		err = runEmailMode()
	case *testEmail:
		err = runTestEmail()
	case *interests:
		err = runInterestMode()
	default:
		err = run()
	}

	if err != nil {
		log.Fatalf("[ERRO] Serviço de promoções encerrado: %v", err)
	}
}

func runEmailMode() error {
	from := os.Getenv("RESEND_FROM")
	recipient := os.Getenv("RESEND_TEST_RECIPIENT")

	client, err := mspromocoes.NewResendClient(
		os.Getenv("RESEND_API_KEY"),
		from,
		recipient,
	)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	configuration := config.Load()

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := database.Connect(startupCtx, configuration.DatabaseURL)
	cancel()
	if err != nil {
		return fmt.Errorf("erro ao conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	repository := mspromocoes.NewEmailRepository(pool)

	log.Print(
		"[INICIADO] Executor de e-mails pendentes; " +
			"restrito ao destinatário de teste configurado",
	)

	return repository.RunEmailWorker(
		ctx,
		from,
		recipient,
		client.Send,
	)
}

func run() error {
	privateKey, err := cryptography.LoadPrivateKey("keys/ms-promocoes/private.pem")
	if err != nil {
		return fmt.Errorf("erro ao carregar chave de Promoções: %w", err)
	}

	promotionService, err := mspromocoes.InitializePromotionService()
	if err != nil {
		return err
	}
	fmt.Println("[SUCESSO] Microsserviço promocoes inicializado com sucesso")

	defer promotionService.Connection.Close()
	defer promotionService.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO PROMOCOES                     ")
	fmt.Println("==================================================================")

	inventoryData, err := inventory.Load("inventory.json")
	if err != nil {
		return err
	}
	if len(inventoryData.Products) == 0 {
		return fmt.Errorf("nenhum produto disponível para gerar promoções")
	}

	for {
		promotion := mspromocoes.GeneratePromotion(inventoryData.Products)

		routingKey, exists := mspromocoes.RoutingKeyForCategory(promotion.Category)
		if !exists {
			log.Printf(
				"[ERRO] Categoria %s sem routing key mapeada, promoção ignorada",
				promotion.Category,
			)
			time.Sleep(10 * time.Second)
			continue
		}

		if _, err := mspromocoes.PublishSignedPromotion(context.Background(), promotionService.Connection, promotion, privateKey); err != nil {
			return fmt.Errorf("erro ao publicar promoção: %w", err)
		}

		log.Printf(
			"[SUCESSO] Promoção publicada: produto=%s desconto=%.1f%% routing_key=%s",
			promotion.ProductName,
			promotion.DiscountPercentage,
			routingKey,
		)

		time.Sleep(10 * time.Second)
	}
}

func runInterestMode() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	configuration := config.Load()

	publicKeys, err := cryptography.LoadPublicKeyRegistry(
		map[string]string{
			events.ProducerPrincipal: "keys/ms-principal/public.pem",
		},
	)
	if err != nil {
		return fmt.Errorf("erro ao carregar chave do Principal: %w", err)
	}

	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	pool, err := database.Connect(startupCtx, configuration.DatabaseURL)
	cancel()
	if err != nil {
		return fmt.Errorf("erro ao conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	repository := mspromocoes.NewInterestRepository(pool)

	return repository.RunInterestConsumer(
		ctx,
		configuration.RabbitMQURL,
		publicKeys,
	)
}

func runTestEmail() error {
	idempotencyKey := os.Getenv("RESEND_TEST_IDEMPOTENCY_KEY")
	if idempotencyKey == "" {
		return fmt.Errorf("configure RESEND_TEST_IDEMPOTENCY_KEY")
	}

	recipient := os.Getenv("RESEND_TEST_RECIPIENT")

	client, err := mspromocoes.NewResendClient(
		os.Getenv("RESEND_API_KEY"),
		os.Getenv("RESEND_FROM"),
		recipient,
	)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	id, err := client.Send(ctx, mspromocoes.EmailMessage{
		To:      recipient,
		Subject: "Event Market — teste de integração",
		Text:    "Este é um e-mail de teste da integração do Event Market com o Resend. Nenhuma promoção automática foi iniciada.",
	}, idempotencyKey)
	if err != nil {
		return err
	}

	log.Printf("[SUCESSO] E-mail aceito pelo Resend: id=%s", id)
	return nil
}
