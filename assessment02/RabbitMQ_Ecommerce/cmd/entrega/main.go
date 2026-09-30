package main

import (
	"fmt"
	"log"

	msentrega "RabbitMQ_Ecommerce/microservices/ms-entrega"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
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
	deliveryService, err := msentrega.InitializeDeliveryService()
	if err != nil {
		return err
	}
	fmt.Println("[SUCESSO] Microsserviço entrega inicializado com sucesso")

	defer deliveryService.Connection.Close()
	defer deliveryService.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO ENTREGA                       ")
	fmt.Println("==================================================================")

	return rabbitmq.ConsumeSignedEvents(
		deliveryService.Channel,
		deliveryService.QueueName,
		deliveryService.PublicKeys,
		func(envelope events.EventEnvelope) error {
			return msentrega.HandleDeliveryEvent(envelope, deliveryService.Channel, deliveryService.PrivateKey)
		},
	)
}
