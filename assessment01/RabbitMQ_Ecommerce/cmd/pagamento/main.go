package main

import (
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/microservices/ms-pagamento"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Println("[ERRO] Erro ao executar o sistema:", err)
		log.Fatal(err)
	}
}

func run() error {
	msPagamento, err := mspagamento.InitMSPagamento()
	if err != nil {
		return err
	}
	fmt.Println("[SUCESSO] Microsserviço pagamento inicializado com sucesso")

	defer msPagamento.Connection.Close()
	defer msPagamento.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO PAGAMENTO                     ")
	fmt.Println("==================================================================")

	return rabbitmq.ConsumeSignedEvents(
		msPagamento.Channel,
		msPagamento.QueueName,
		msPagamento.PublicKeys,
		func(envelope events.EventEnvelope) error {
			return mspagamento.HandlePaymentEvent(envelope, msPagamento.Channel, msPagamento.PrivateKey)
		},
	)
}
