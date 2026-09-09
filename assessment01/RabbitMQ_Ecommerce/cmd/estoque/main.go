package main

import (
	"log"
	"fmt"

	"RabbitMQ_Ecommerce/microsservices/ms-estoque"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"RabbitMQ_Ecommerce/utils/events"
)

func main() {
	if err := run(); err != nil {
		log.Println("[ERRO] Erro ao executar o sistema:", err)
		log.Fatal(err)
	}
}

func run() error {
	msEstoque, err := msestoque.InitMSEstoque()
	if err != nil {
		return err
	}

	fmt.Println("[SUCESSO] Microsserviço estoque inicializado com sucesso")

	defer msEstoque.Connection.Close()
	defer msEstoque.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("                      MICROSSERVIÇO ESTOQUE                       ")
	fmt.Println("==================================================================")

	stock := map[string]int{
		"product_001": 10,
		"product_002": 5,
	}

	reservations := make(map[string]events.Order)

	return rabbitmq.ConsumeEvents(
		msEstoque.Channel,
		msEstoque.QueueName,
		func(envelope events.EventEnvelope) error {
			return msestoque.HandleStockEvent(envelope, stock, reservations, msEstoque.Channel)
		},
	)
}
