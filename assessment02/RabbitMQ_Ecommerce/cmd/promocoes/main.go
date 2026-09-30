package main

import (
	"fmt"
	"log"
	"time"

	mspromocoes "RabbitMQ_Ecommerce/microservices/ms-promocoes"
	"RabbitMQ_Ecommerce/utils/inventory"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf(
			"[ERRO] Erro ao executar o sistema: %v",
			err,
		)
	}
}

func run() error {
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

		if err := mspromocoes.PublishPromotion(promotionService.Channel, promotion, routingKey); err != nil {
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
