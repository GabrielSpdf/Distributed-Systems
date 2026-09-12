package msestoque

import (
	"encoding/json"
	"fmt"
	"log"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"RabbitMQ_Ecommerce/utils/inventory"

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

func ReserveStock(
	stock map[string]int,
	reservations map[string]events.Order,
	order events.Order,
) (bool, error) {
	if order.OrderID == "" {
		return true, fmt.Errorf("pedido sem identificador")
	}

	if reservations == nil {
		return true, fmt.Errorf("mapa de reservas não inicializado")
	}

	if _, exists := reservations[order.OrderID]; exists {
		return true, fmt.Errorf("pedido já reservado: %s", order.OrderID)
	}

	if len(order.Items) == 0 {
		return true, fmt.Errorf(
			"nenhum item fornecido para o pedido %s",
			order.OrderID,
		)
	}

	groupedItems := make(map[string]events.OrderItem, len(order.Items))
	itemOrder := make([]string, 0, len(order.Items))

	// Agrupa produtos repetidos sem alterar o estoque.
	for _, item := range order.Items {
		if item.Product.ID == "" {
			return true, fmt.Errorf("produto sem identificador no pedido %s", order.OrderID)
		}

		if item.Quantity <= 0 {
			return true, fmt.Errorf(
				"quantidade inválida para o produto %s: %d",
				item.Product.ID,
				item.Quantity,
			)
		}

		groupedItem, exists := groupedItems[item.Product.ID]
		if !exists {
			groupedItems[item.Product.ID] = item
			itemOrder = append(itemOrder, item.Product.ID)
			continue
		}

		groupedItem.Quantity += item.Quantity
		groupedItems[item.Product.ID] = groupedItem
	}

	// Verifica todos os produtos antes de realizar qualquer baixa.
	for _, productID := range itemOrder {
		item := groupedItems[productID]

		available, exists := stock[productID]
		if !exists {
			return false, nil
		}

		if item.Quantity > available {
			return false, nil
		}
	}

	// Copia os itens para que a reserva mantenha seus próprios dados.
	reservedOrder := order
	reservedOrder.Items = make([]events.OrderItem, len(order.Items))
	copy(reservedOrder.Items, order.Items)
	reservedOrder.Status = events.StatusStockReserved

	// Todos os itens estão disponíveis: realiza a baixa.
	for _, productID := range itemOrder {
		stock[productID] -= groupedItems[productID].Quantity
	}

	reservations[order.OrderID] = reservedOrder

	return true, nil
}

func ReleaseStock(
	stock map[string]int,
	reservations map[string]events.Order,
	orderID string,
) error {
	if orderID == "" {
		return fmt.Errorf("pedido sem identificador")
	}

	if reservations == nil {
		return fmt.Errorf("mapa de reservas não inicializado")
	}

	reservedOrder, exists := reservations[orderID]
	if !exists {
		return nil
	}

	if stock == nil {
		return fmt.Errorf("mapa de estoque não inicializado")
	}

	for _, item := range reservedOrder.Items {
		stock[item.Product.ID] += item.Quantity
	}

	delete(reservations, orderID)

	return nil
}

func HandleStockEvent(
	envelope events.EventEnvelope,
	stock map[string]int,
	reservations map[string]events.Order,
	channel *amqp.Channel,
	inventoryFileName string,
) error {
	switch envelope.EventType {
	case events.PedidoCriado:
		var payload events.Order

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pedido.criado: %w",
				err,
			)
		}

		isAvailable, err := ReserveStock(
			stock,
			reservations,
			payload,
		)
		if err != nil {
			return fmt.Errorf(
				"não foi possível reservar o pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

		if !isAvailable {
			log.Printf(
				"[ERRO] Estoque insuficiente para o pedido %s",
				payload.OrderID,
			)

			err := PublishStockUnavailable(
				channel,
				payload.OrderID,
			)
			if err != nil {
				return fmt.Errorf(
					"erro ao publicar pedido.estoque_indisponivel: %w",
					err,
				)
			}
		} else {
			if err := inventory.SaveStock(
				inventoryFileName,
				stock,
			); err != nil {
				return fmt.Errorf(
					"erro ao persistir baixa do pedido %s: %w",
					payload.OrderID,
					err,
				)
			}

			err := PublishStockOk(
				channel,
				payload.OrderID,
			)
			if err != nil {
				return fmt.Errorf(
					"erro ao publicar pedido.estoque_ok: %w",
					err,
				)
			}

			log.Printf(
				"[SUCESSO] Pedido %s reservado com sucesso",
				payload.OrderID,
			)
		}

	case events.PedidoExcluido:
		var payload events.OrderReferencePayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pedido.excluido: %w",
				err,
			)
		}

		if err := ReleaseStock(
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

		if err := inventory.SaveStock(
			inventoryFileName,
			stock,
		); err != nil {
			return fmt.Errorf(
				"erro ao persistir devolução do pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

		log.Printf(
			"[SUCESSO] Exclusão realizada com sucesso para o pedido %s",
			payload.OrderID,
		)

	default:
		return fmt.Errorf(
			"tipo de evento inesperado no Estoque: %s",
			envelope.EventType,
		)
	}

	return nil
}

func PublishStockOk(channel *amqp.Channel, orderID string) error {
	fmt.Println("==================================================================")
	fmt.Printf("                  ESTOQUE VERIFICADO - PEDIDO - %s               \n", orderID)
	fmt.Println("==================================================================")

	payload := events.OrderReferencePayload{
		OrderID: orderID,
	}

	envelope, err := misc.MountEnvelope(
		payload,
		events.PedidoEstoqueOk,
		"ms-estoque",
		"signature",
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.PedidoEstoqueOk,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}

func PublishStockUnavailable(channel *amqp.Channel, orderID string) error {
	fmt.Println("==================================================================")
	fmt.Printf("                  ESTOQUE INDISPONÍVEL - PEDIDO - %s               \n", orderID)
	fmt.Println("==================================================================")

	payload := events.OrderReferencePayload{
		OrderID: orderID,
	}

	envelope, err := misc.MountEnvelope(
		payload,
		events.EstoqueIndisponivel,
		"ms-estoque",
		"signature",
	)
	if err != nil {
		return fmt.Errorf("erro ao montar envelope: %w", err)
	}

	if err := rabbitmq.PublishEvent(
		channel,
		events.ExchangeEcommerce,
		events.EstoqueIndisponivel,
		envelope,
	); err != nil {
		return fmt.Errorf("erro ao enviar evento: %w", err)
	}

	return nil
}
