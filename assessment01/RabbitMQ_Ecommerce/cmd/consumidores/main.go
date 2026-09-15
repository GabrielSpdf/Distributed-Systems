package main

import (
	"fmt"
	"log"
	"os"

	msconsumidores "RabbitMQ_Ecommerce/microservices/ms-consumidores"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("informe o tipo do consumidor: go run . 1 ou go run . 2")
	}

	consumerType := os.Args[1]

	if consumerType != "1" && consumerType != "2" {
		log.Fatal("tipo de consumidor inválido: use 1 ou 2")
	}

	if err := run(consumerType); err != nil {
		log.Fatal("[ERRO] Erro ao executar o consumidor:", err)
	}
}

func run(consumerType string) error {
	var promotionConsumer *msconsumidores.Runtime
	var err error

	if consumerType == "1" {
		promotionConsumer, err = msconsumidores.InitializePromotionConsumer(false)
	} else {
		promotionConsumer, err = msconsumidores.InitializePromotionConsumer(true)
	}

	if err != nil {
		return err
	}

	fmt.Printf("[SUCESSO] Microsserviço consumidor %s inicializado com sucesso \n", consumerType)

	defer promotionConsumer.Connection.Close()
	defer promotionConsumer.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Printf("                      MICROSSERVIÇO CONSUMIDOR - %s               \n", consumerType)
	fmt.Println("==================================================================")

	return rabbitmq.ConsumeEvents(
		promotionConsumer.Channel,
		promotionConsumer.QueueName,
		func(envelope events.EventEnvelope) error {
			return msconsumidores.HandleConsumerEvent(envelope)
		},
	)
}
