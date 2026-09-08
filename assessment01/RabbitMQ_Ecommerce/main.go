package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
)

func main() {
	orderData := events.OrderCreatedPayload{
		OrderID:    "PED-001",
		CustomerID: "CLI-001",
		Items: []events.OrderItem{
			{
				ProductID: "PROD-001",
				Name:      "Teclado",
				Quantity:  2,
				UnitPrice: 150,
			},
		},
		Total: 300,
	}

	dataJSON, err := json.Marshal(orderData)
	if err != nil {
		log.Fatal(err)
	}

	envelope := events.EventEnvelope{
		EventID:   "EVENTO-001",
		EventType: events.PedidoCriado,
		Producer:  "principal",
		Timestamp: time.Now(),
		Payload:      dataJSON,
		Signature: "",
	}

	eventJSON, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(eventJSON))
}