package msprincipal

import (
	"fmt"
	"log"
	"strconv"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"RabbitMQ_Ecommerce/utils/misc"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Runtime struct {
	Connection *amqp.Connection
	Channel    *amqp.Channel
	QueueName  string
}

func InitMSPrincipal() (
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

	principalQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueuePrincipal,
	)
	if err != nil {
		channel.Close()
		connection.Close()
		return nil, err
	}
	log.Println("[SUCESSO] Fila principal declarada")
	
	for _, routingKey := range []string{
		events.PedidoEstoqueOk,
		events.EstoqueIndisponivel,
		events.PagamentoAprovado,
		events.PagamentoRecusado,
		events.PedidoEnviado,
	} {
		if err := rabbitmq.BindQueue(
			channel,
			principalQueue.Name,
			routingKey,
			events.ExchangeEcommerce,
		); err != nil {
			channel.Close()
			connection.Close()
			return nil, err
		}
		log.Println("[SUCESSO] Fila principal ligada à exchange Ecommerce e vinculada ao roteamento", routingKey)
	}

	runTime := &Runtime{
		Connection: connection,
		Channel:    channel,
		QueueName:  principalQueue.Name,
	}

	return runTime, nil
}

func PublishCreateOrder(channel *amqp.Channel, orderID int) error {
	fmt.Println("==================================================================")
	fmt.Printf("                         CRIANDO PEDIDO - %d               \n", orderID)
	fmt.Println("==================================================================")

	order := events.OrderCreatedPayload{
			OrderID:    strconv.Itoa(orderID),
			CustomerID: "user_001",
			Items: []events.OrderItem{
				{
					Product: events.Product{
						ID:       "product_001",
						Name:     "Produto 1",
						Category: "Categoria A",
						Price:    10,
					},
					Quantity:  2,
					Price: 20,
				},
				{
					Product: events.Product{
						ID:       "product_002",
						Name:     "Produto 2",
						Category: "Categoria B",
						Price:    10,
					},
					Quantity:  1,
					Price: 10,
				},
			},
			Total: 30,
		}

		envelope, err := misc.MountEnvelope(
			order,
			events.PedidoCriado,
			"ms-principal",
			"signature",
		)		
		if err != nil {
			return fmt.Errorf("erro ao montar envelope: %w", err)
		}

		if err := rabbitmq.PublishEvent(
			channel,
			events.ExchangeEcommerce,
			events.PedidoCriado,
			envelope,
		); err != nil {
			return fmt.Errorf("erro ao enviar evento: %w", err)
		}

	return nil
}

func PublishDeleteOrder(channel *amqp.Channel, orderID int) error {
	fmt.Println("==================================================================")
	fmt.Printf("                         EXCLUINDO PEDIDO - %d               \n", orderID)
	fmt.Println("==================================================================")

	payload := events.OrderReferencePayload{
		OrderID: strconv.Itoa(orderID),
	}

	envelope, err := misc.MountEnvelope(
		payload,
		events.PedidoExcluido,
		"ms-principal",
		"signature",
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PedidoExcluido,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}

func ShowOrders() (int, error) {
	fmt.Println("==================================================================")
	fmt.Println("                         LISTA DE PEDIDOS                       ")
	fmt.Println("==================================================================")

	var order int
		fmt.Print("Selecione um pedido para deleção: ")
		_, err := fmt.Scan(&order)
		if err != nil {
			return 0, err
		}

	return order, nil
}