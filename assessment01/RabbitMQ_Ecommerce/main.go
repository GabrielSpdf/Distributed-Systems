package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf(
			"uso: go run . <criar|excluir> <order_id>",
		)
	}

	action := os.Args[1]
	orderID := os.Args[2]

	if orderID == "" {
		return fmt.Errorf("identificador do pedido não pode ser vazio")
	}

	var (
		eventType   string
		payloadJSON []byte
		err         error
	)

	switch action {
	case "criar":
		order := events.OrderCreatedPayload{
			OrderID:    orderID,
			CustomerID: "user_001",
			Items: []events.OrderItem{
				{
					ProductID: "prod_001",
					Name:      "Produto A",
					Quantity:  2,
					UnitPrice: 10,
				},
				{
					ProductID: "prod_002",
					Name:      "Produto B",
					Quantity:  1,
					UnitPrice: 20,
				},
			},
			Total: 40,
		}

		eventType = events.PedidoCriado
		payloadJSON, err = json.Marshal(order)

	case "excluir":
		reference := events.OrderReferencePayload{
			OrderID: orderID,
		}

		eventType = events.PedidoExcluido
		payloadJSON, err = json.Marshal(reference)

	default:
		return fmt.Errorf(
			"ação desconhecida: %s; utilize criar ou excluir",
			action,
		)
	}

	if err != nil {
		return fmt.Errorf("erro ao serializar payload: %w", err)
	}

	// Gera um identificador aleatório para esta publicação.
	var eventIDBytes [16]byte
	if _, err := rand.Read(eventIDBytes[:]); err != nil {
		return fmt.Errorf("erro ao gerar identificador do evento: %w", err)
	}

	envelope := events.EventEnvelope{
		EventID:   hex.EncodeToString(eventIDBytes[:]),
		EventType: eventType,
		Producer:  "EcommerceService",
		Timestamp: time.Now(),
		Payload:   payloadJSON,
		Signature: "",
	}

	connection, err := rabbitmq.Connect()
	if err != nil {
		return err
	}
	defer connection.Close()

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		return err
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		eventType,
		envelope,
	); err != nil {
		return err
	}

	log.Printf(
		"[✓] Publicação enviada: evento=%s tipo=%s pedido=%s",
		envelope.EventID,
		envelope.EventType,
		orderID,
	)

	return nil
}