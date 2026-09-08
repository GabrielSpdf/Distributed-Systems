package main

import (
	"encoding/json"
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

	order := events.OrderCreatedPayload{
		OrderID:    "12345",
		CustomerID: "user_001",
		Items: []events.OrderItem{
			{
				ProductID: "prod_001",
				Name:      "Produto A",
				Quantity:  2,
				UnitPrice: 10.0,
			},
			{
				ProductID: "prod_002",
				Name:      "Produto B",
				Quantity:  1,
				UnitPrice: 20.0,
			},
		},
		Total: 40.0,
	}

	payloadJSON, err := json.Marshal(order)
	if err != nil {
		log.Fatal(err)
	}

	envelope := events.EventEnvelope{
		EventID:   "Evt001",
		EventType: events.PedidoCriado,
		Producer:  "EcommerceService",
		Timestamp: time.Now(),
		Payload:   payloadJSON,
		Signature: "",
	}

	err = rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PedidoCriado,
		envelope,
	)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf(
		"[✓] Evento %s publicado na exchange %s com routing key %s",
		envelope.EventID,
		events.ExchangeEcommerce,
		events.PedidoCriado,
	)
}
