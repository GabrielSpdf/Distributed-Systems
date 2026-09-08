package main

import (
	"log"

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
}