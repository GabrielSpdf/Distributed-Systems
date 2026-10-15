package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mspromocoes "RabbitMQ_Ecommerce/microservices/ms-promocoes"
	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/inventory"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	once := flag.Bool("once", false, "Publicar uma oferta e encerrar")
	interval := flag.Duration("interval", 60*time.Second, "Intervalo entre ofertas")
	category := flag.String(
		"category", "", "Categoria: alimentos, limpeza ou eletronicos",
	)
	repeat := flag.Int("repeat", 1, "Quantidade de cópias do mesmo evento")
	flag.Parse()

	if err := run(*once, *interval, *category, *repeat); err != nil {
		log.Fatalf("[ERRO] %v", err)
	}
}

func run(
	once bool,
	interval time.Duration,
	category string,
	repeat int,
) error {
	if repeat < 1 || repeat > 10 {
		return fmt.Errorf("repetições devem estar entre 1 e 10")
	}

	if interval <= 0 {
		return fmt.Errorf("intervalo deve ser positivo")
	}

	category = strings.ToLower(strings.TrimSpace(category))
	categoryNames := map[string]string{
		"alimentos":   "Alimentos",
		"limpeza":     "Limpeza",
		"eletronicos": "Eletrônicos",
	}
	if category != "" && categoryNames[category] == "" {
		return fmt.Errorf("categoria inválida")
	}

	data, err := inventory.Load("inventory.json")
	if err != nil {
		return err
	}

	var products []events.Product
	for _, product := range data.Products {
		if category == "" || product.Category == categoryNames[category] {
			products = append(products, product)
		}
	}
	if len(products) == 0 {
		return fmt.Errorf("nenhum produto disponível para o sorteio")
	}

	privateKey, err := cryptography.LoadPrivateKey(
		"keys/ms-promocoes/private.pem",
	)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer stop()

	connection, err := amqp.DialConfig(
		config.Load().RabbitMQURL,
		amqp.Config{
			Heartbeat: 10 * time.Second,
			Dial:      amqp.DefaultDial(5 * time.Second),
		},
	)
	if err != nil {
		return err
	}
	defer connection.Close()

	stopClose := context.AfterFunc(ctx, func() {
		_ = connection.Close()
	})
	defer stopClose()

	channel, err := connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	for {
		if ctx.Err() != nil {
			return nil
		}

		promotion := mspromocoes.GeneratePromotion(products)
		eventID, err := mspromocoes.PublishSignedPromotion(
			channel, promotion, privateKey, repeat,
		)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}

		log.Printf(
			"[PUBLICADO] Oferta=%s produto=%s categoria=%s preço=%.2f desconto=%.2f%%",
			eventID,
			promotion.ProductName,
			promotion.Category,
			promotion.PromotionalPrice,
			promotion.DiscountPercentage,
		)

		if once {
			return nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
