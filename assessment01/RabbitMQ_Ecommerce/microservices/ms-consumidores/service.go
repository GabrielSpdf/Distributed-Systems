package msconsumidores

import (
	"encoding/json"
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Runtime struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
	QueueName  string
}

func InitMSConsumer(allCategories bool) (
	*Runtime,
	error,
) {
	connection, err := rabbitmq.Connect()
	if err != nil {
		return nil, err
	}
	log.Println("[SUCESSO] Conexão estabelecida com o RabbitMQ")

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Canal RabbitMQ aberto")

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Exchanges declaradas")

	var msg string
	var queueName string
	var eventsType []string

	if allCategories {
		queueName = events.QueueConsumidor2
		eventsType = []string{
			events.AllPromotions,
		}
		msg = "[SUCESSO] Fila Consumidor 2 declarada para consumir eventos de todas as categorias"
	} else {
		queueName = events.QueueConsumidor1
		eventsType = []string{
			events.PromocaoCategoriaA,
			events.PromocaoCategoriaB,
		}
		msg = "[SUCESSO] Fila Consumidor 1 declarada para consumir eventos da categorias Alimentos e Limpeza"
	}

	consumerQueue, err := rabbitmq.DeclareQueue(
		channel,
		queueName,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println(msg)

	for _, routingKey := range eventsType {
		if err := rabbitmq.BindQueue(
			channel,
			consumerQueue.Name,
			routingKey,
			events.ExchangePromocoes,
		); err != nil {
			channel.Close()
			connection.Close()
			return nil, err
		}
		log.Println("[SUCESSO] Fila estoque ligada à exchange Promocoes e vinculada ao roteamento", routingKey)
	}

	runTime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  consumerQueue.Name,
	}

	return runTime, nil
}

func HandleConsumerEvent(
	envelope events.EventEnvelope,
	channel *amqp.Channel,
) error {
	switch envelope.EventType {
	case events.PromocaoCategoriaA, events.PromocaoCategoriaB, events.PromocaoCategoriaC:
		var payload events.PromotionPayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar promoção: %w",
				err,
			)
		}

		fmt.Println("==================================================================")
		log.Printf(
			"[SUCESSO] Evento recebido: evento=%s tipo=%s",
			envelope.EventID,
			envelope.EventType,
		)

		fmt.Println("==================================================================")
		fmt.Printf("                      PROMOÇÃO DA CATEGORIA %s                     \n", payload.Category)
		fmt.Println("==================================================================")

		fmt.Printf(
			"ID do Produto: %s \nProduto: %s \nPreço Original: %.2f \nDesconto: %.2f%% \nPreço com Desconto: %.2f \n",
			payload.ProductID,
			payload.ProductName,
			payload.OriginalPrice,
			payload.DiscountPercentage,
			payload.PromotionalPrice,
		)

	default:
		return fmt.Errorf(
			"tipo de evento inesperado no Consumidor: %s",
			envelope.EventType,
		)
	}

	return nil
}
