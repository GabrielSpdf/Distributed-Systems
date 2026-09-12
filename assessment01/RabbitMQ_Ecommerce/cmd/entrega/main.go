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
		log.Println("[ERRO] Erro ao executar o sistema:", err)
		log.Fatal(err)
	}
}

func run() error {
	msEntrega, err := msentrega.InitMSEntrega()
	if err != nil {
		return err
	}
	fmt.Println("[SUCESSO] Microsserviço pagamento inicializado com sucesso")

	defer msEntrega.Connection.Close()
	defer msEntrega.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO ENTREGA                       ")
	fmt.Println("==================================================================")

	return rabbitmq.ConsumeEvents(
		msEntrega.Channel,
		msEntrega.QueueName,
		func(envelope events.EventEnvelope) error {
			return msentrega.HandleDeliveryEvent(envelope, msEntrega.Channel)
		},
	)
}
