package main

import (
	"fmt"
	"log"
	"os"

	"RabbitMQ_Ecommerce/microservices/ms-consumidores"
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
	var msConsumer *msconsumidores.Runtime
	var err error

	if consumerType == "1" {
		msConsumer, err = msconsumidores.InitMSConsumer(false)
	} else {
		msConsumer, err = msconsumidores.InitMSConsumer(true)
	}

	if err != nil {
		return err
	}
	fmt.Println("[SUCESSO] Microsserviço entrega inicializado com sucesso")

	defer msConsumer.Connection.Close()
	defer msConsumer.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Printf("                      MICROSSERVIÇO CONSUMIDOR - %s               \n", consumerType)
	fmt.Println("==================================================================")

	return rabbitmq.ConsumeEvents(
		msConsumer.Channel,
		msConsumer.QueueName,
		func(envelope events.EventEnvelope) error {
			return msconsumidores.HandleConsumerEvent(envelope, msConsumer.Channel)
		},
	)
}
