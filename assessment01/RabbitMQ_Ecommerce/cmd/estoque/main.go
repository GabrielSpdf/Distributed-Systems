package main

import (
	"log"
	"fmt"

	"RabbitMQ_Ecommerce/microservices/ms-estoque"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/inventory"
	"RabbitMQ_Ecommerce/microservices/ms-principal"
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

	inventoryData, err := inventory.Load("inventory.json")
	if err != nil {
		return err
	}

	stock := inventoryData.Stock

	ordersData, err := msprincipal.LoadOrders("data/orders.json")
	if err != nil {
		return err
	}

	reservations := make(map[string]events.Order)

	for _, order := range ordersData.Orders {
		reservations[order.OrderID] = order
	}

	return rabbitmq.ConsumeEvents(
		msEstoque.Channel,
		msEstoque.QueueName,
		func(envelope events.EventEnvelope) error {
			return msestoque.HandleStockEvent(envelope, stock, reservations, msEstoque.Channel, "inventory.json")
		},
	)
}
