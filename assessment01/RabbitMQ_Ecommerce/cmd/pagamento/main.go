package main

import (
	"fmt"
	"log"

	mspagamento "RabbitMQ_Ecommerce/microservices/ms-pagamento"
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
	paymentService, err := mspagamento.InitializePaymentService()
	if err != nil {
		return err
	}
	fmt.Println("[SUCESSO] Microsserviço pagamento inicializado com sucesso")

	defer paymentService.Connection.Close()
	defer paymentService.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO PAGAMENTO                     ")
	fmt.Println("==================================================================")

	return rabbitmq.ConsumeSignedEvents(
		paymentService.Channel,
		paymentService.QueueName,
		paymentService.PublicKeys,
		func(envelope events.EventEnvelope) error {
			return mspagamento.HandlePaymentEvent(envelope, paymentService.Channel, paymentService.PrivateKey)
		},
	)
}
