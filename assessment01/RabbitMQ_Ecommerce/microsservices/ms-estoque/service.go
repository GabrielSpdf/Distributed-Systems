package msestoque

import (
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

func InitMSEstoque() (
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

	stockQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueueEstoque,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila estoque declarada")
	
	for _, routingKey := range []string{
		events.PedidoCriado,
		events.PedidoExcluido,
	} {
		if err := rabbitmq.BindQueue(
			channel,
			stockQueue.Name,
			routingKey,
			events.ExchangeEcommerce,
		); err != nil {
			channel.Close()
			connection.Close()
			return nil, err
		}
		log.Println("[SUCESSO] Fila estoque ligada à exchange Ecommerce e vinculada ao roteamento", routingKey)
	}

	runTime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  stockQueue.Name,
	}

	return runTime, nil
}