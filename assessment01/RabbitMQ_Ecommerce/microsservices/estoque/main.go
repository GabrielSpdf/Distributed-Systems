package main

import (
	"encoding/json"
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	// Estabelece a conexão TCP com o RabbitMQ.
	connection, err := rabbitmq.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer connection.Close()

	log.Println("[✓] Serviço Estoque conectado ao RabbitMQ")

	// Abre um canal lógico dentro da conexão.
	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		log.Fatal(err)
	}
	defer channel.Close()

	log.Println("[✓] Canal RabbitMQ aberto")

	// Garante que as exchanges do sistema existam.
	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		log.Fatal(err)
	}

	// Garante que a fila do Estoque exista.
	stockQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueueEstoque,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Vincula pedido.criado à fila do Estoque.
	if err := rabbitmq.BindQueue(
		channel,
		stockQueue.Name,
		events.PedidoCriado,
		events.ExchangeEcommerce,
	); err != nil {
		log.Fatal(err)
	}

	log.Printf(
		"[✓] Serviço Estoque aguardando eventos na fila %s",
		stockQueue.Name,
	)

	// ConsumeEvents permanece executando enquanto o canal estiver aberto.
	if err := rabbitmq.ConsumeEvents(
		channel,
		stockQueue.Name,
		handleOrderCreated,
	); err != nil {
		log.Fatal(err)
	}
}

// handleOrderCreated processa um evento pedido.criado recebido pelo Estoque.
func handleOrderCreated(envelope events.EventEnvelope) error {
	if envelope.EventType != events.PedidoCriado {
		return fmt.Errorf(
			"tipo de evento inesperado: recebido %s, esperado %s",
			envelope.EventType,
			events.PedidoCriado,
		)
	}

	var order events.OrderCreatedPayload

	if err := json.Unmarshal(envelope.Payload, &order); err != nil {
		return fmt.Errorf(
			"erro ao desserializar payload do pedido: %w",
			err,
		)
	}

	log.Printf("[→] Evento recebido: %s", envelope.EventType)
	log.Printf("[→] ID do evento: %s", envelope.EventID)
	log.Printf("[→] Produtor: %s", envelope.Producer)
	log.Printf("[→] Pedido: %s", order.OrderID)
	log.Printf("[→] Cliente: %s", order.CustomerID)
	log.Printf("[→] Total: R$ %.2f", order.Total)

	for _, item := range order.Items {
		log.Printf(
			"[→] Produto: %s | Quantidade: %d | Preço unitário: R$ %.2f",
			item.Name,
			item.Quantity,
			item.UnitPrice,
		)
	}

	// Futuramente, a verificação e a reserva de estoque entrarão aqui.
	log.Printf("[✓] Pedido %s processado pelo Estoque", order.OrderID)

	return nil
}
