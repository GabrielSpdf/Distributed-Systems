package rabbitmq

import (
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeTypeDirect = "direct"
	ExchangeTypeTopic  = "topic"
)

func DeclareExchange(
	channel *amqp.Channel,
	name string,
	exchangeType string,
) error {
	err := channel.ExchangeDeclare(
		name,
		exchangeType,
		true,  // Durable
		false, // AutoDelete
		false, // Internal
		false, // NoWait
		nil,   // Arguments
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao declarar exchange %s do tipo %s: %w",
			name,
			exchangeType,
			err,
		)
	}

	return nil
}

func DeclareExchanges(channel *amqp.Channel) error {
	err := DeclareExchange(
		channel,
		events.ExchangeEcommerce,
		ExchangeTypeDirect,
	)
	if err != nil {
		return err
	}
	log.Println("[SUCESSO] Exchange Ecommerce [direct] declarada com sucesso")

	err = DeclareExchange(
		channel,
		events.ExchangePromocoes,
		ExchangeTypeTopic,
	)
	if err != nil {
		return err
	}
	log.Println("[SUCESSO] Exchange Promoções [topic] declarada com sucesso")

	return nil
}
