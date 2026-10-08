package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"RabbitMQ_Ecommerce/utils/config"
	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	orderID := flag.String("order", "", "Identificador público do pedido")
	customerID := flag.String("customer", "", "UUID do cliente")
	total := flag.Float64("total", 0, "Total do pedido")
	repeat := flag.Int("repeat", 1, "Quantidade de envios do mesmo envelope")
	tamper := flag.Bool("tamper", false, "Adulterar payload após assinatura")
	flag.Parse()

	if strings.TrimSpace(*orderID) == "" ||
		strings.TrimSpace(*customerID) == "" ||
		*total <= 0 || math.IsNaN(*total) || math.IsInf(*total, 0) ||
		*repeat < 1 || *repeat > 10 {
		return fmt.Errorf("informe pedido, cliente, total positivo e repeat entre 1 e 10")
	}

	privateKey, err := cryptography.LoadPrivateKey(
		"keys/ms-estoque/private.pem",
	)
	if err != nil {
		return err
	}

	envelope, err := misc.BuildSignedEnvelope(
		events.WebStockConfirmedPayload{
			OrderID:    *orderID,
			CustomerID: *customerID,
			Total:      *total,
			Currency:   "BRL",
			ReservedAt: time.Now().UTC(),
		},
		events.OrderStockConfirmed,
		events.ProducerStock,
		privateKey,
	)
	if err != nil {
		return err
	}

	if *tamper {
		var changedPayload events.WebStockConfirmedPayload
		if err := json.Unmarshal(envelope.Payload, &changedPayload); err != nil {
			return err
		}

		changedPayload.Total += 1

		envelope.Payload, err = json.Marshal(changedPayload)
		if err != nil {
			return err
		}
	}

	connection, err := rabbitmq.ConnectURL(config.Load().RabbitMQURL)
	if err != nil {
		return err
	}
	defer connection.Close()

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := rabbitmq.DeclareExchange(
		channel, events.ExchangeEcommerce, rabbitmq.ExchangeTypeDirect,
	); err != nil {
		return err
	}

	if err := channel.Confirm(false); err != nil {
		return err
	}
	confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))

	for index := 0; index < *repeat; index++ {
		if err := rabbitmq.PublishEvent(
			channel,
			events.ExchangeEcommerce,
			events.OrderStockConfirmed,
			envelope,
		); err != nil {
			return err
		}

		select {
		case confirmation, open := <-confirmations:
			if !open || !confirmation.Ack {
				return fmt.Errorf("publicação não confirmada pelo RabbitMQ")
			}
		case <-time.After(5 * time.Second):
			return fmt.Errorf("tempo esgotado aguardando confirmação")
		}
	}

	log.Printf(
		"Evento %s enviado %d vez(es) para %s",
		envelope.EventID, *repeat, *orderID,
	)
	return nil
}
