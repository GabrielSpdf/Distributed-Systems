package main

import (
	"fmt"
	"log"

	msestoque "RabbitMQ_Ecommerce/microservices/ms-estoque"
	msprincipal "RabbitMQ_Ecommerce/microservices/ms-principal"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/inventory"
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
	stockService, err := msestoque.InitializeStockService()
	if err != nil {
		return err
	}

	fmt.Println("[SUCESSO] Microsserviço estoque inicializado com sucesso")

	defer stockService.Connection.Close()
	defer stockService.Channel.Close()

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
		if order.Status == events.StatusStockReserved ||
			order.Status == events.StatusPaymentApproved ||
			order.Status == events.StatusShipped {
			reservations[order.OrderID] = order
		}
	}

	return rabbitmq.ConsumeSignedEvents(
		stockService.Channel,
		stockService.QueueName,
		stockService.PublicKeys,
		func(envelope events.EventEnvelope) error {
			return msestoque.HandleStockEvent(envelope, stock, reservations, stockService.Channel, stockService.PrivateKey, "inventory.json")
		},
	)
}
