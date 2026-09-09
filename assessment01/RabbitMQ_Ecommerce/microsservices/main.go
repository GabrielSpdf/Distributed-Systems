package main

import (
	"encoding/json"
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/microsservices/ms-estoque"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run inicializa o serviço e mantém o consumo dos eventos.
func run() error {
	// Os mapas são criados uma única vez e permanecem entre as mensagens.
	stock := map[string]int{
		"prod_001": 10,
		"prod_002": 5,
	}

	reservations := make(map[string]events.Order)

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

	stockQueue, err := rabbitmq.DeclareQueue(
		channel,
		events.QueueEstoque,
	)
	if err != nil {
		return err
	}

	// A mesma fila recebe os dois tipos de evento.
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
			return err
		}
	}

	log.Printf(
		"[✓] Estoque aguardando eventos na fila %s",
		stockQueue.Name,
	)

	return rabbitmq.ConsumeEvents(
		channel,
		stockQueue.Name,
		func(envelope events.EventEnvelope) error {
			return handleStockEvent(envelope, stock, reservations)
		},
	)
}

// handleStockEvent encaminha cada evento para a operação de estoque correspondente.
func handleStockEvent(
	envelope events.EventEnvelope,
	stock map[string]int,
	reservations map[string]events.Order,
) error {
	switch envelope.EventType {
	case events.PedidoCriado:
		var payload events.OrderCreatedPayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pedido.criado: %w",
				err,
			)
		}

		order := events.Order{
			ID:         payload.OrderID,
			CustomerID: payload.CustomerID,
			Items:      payload.Items,
			Total:      payload.Total,
			Status:     events.StatusCreated,
		}

		if err := msestoque.ReserveStock(
			stock,
			reservations,
			order,
		); err != nil {
			return fmt.Errorf(
				"não foi possível reservar o pedido %s: %w",
				order.ID,
				err,
			)
		}

		log.Printf(
			"[✓] Pedido %s possui reserva registrada",
			order.ID,
		)

		// Etapa seguinte: publicar pedido.estoque_ok.
		

	case events.PedidoExcluido:
		var payload events.OrderReferencePayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pedido.excluido: %w",
				err,
			)
		}

		if err := msestoque.ReleaseStock(
			stock,
			reservations,
			payload.OrderID,
		); err != nil {
			return fmt.Errorf(
				"erro ao liberar reserva do pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

		log.Printf(
			"[✓] Exclusão tratada para o pedido %s",
			payload.OrderID,
		)

	default:
		return fmt.Errorf(
			"tipo de evento inesperado no Estoque: %s",
			envelope.EventType,
		)
	}

	log.Printf("Estoque atual: %v", stock)
	log.Printf("Reservas ativas: %d", len(reservations))

	return nil
}