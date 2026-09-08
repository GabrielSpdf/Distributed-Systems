package main

import (
	"log"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	connection, err := rabbitmq.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close()

	log.Println("[✓] Conectado ao RabbitMQ")

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		log.Fatal(err)
	}
	defer channel.Close()

	log.Println("[✓] Canal RabbitMQ aberto")

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		log.Fatal(err)
	}

	log.Println("[✓] Exchanges declaradas com sucesso")

	stockQueue, err := rabbitmq.DeclareQueue(channel, events.QueueEstoque)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("[✓] Fila %s declarada com sucesso", stockQueue.Name)

	err = rabbitmq.BindQueue(
		channel,
		stockQueue.Name,
		events.PedidoCriado,
		events.ExchangeEcommerce,
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("[✓] Fila %s vinculada à exchange %s com a chave de roteamento %s",
		stockQueue.Name,
		events.ExchangeEcommerce,
		events.PedidoCriado,
	)
}
