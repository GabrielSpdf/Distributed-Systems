package msprincipal

import (
	"fmt"
	"log"
	"encoding/json"
	"text/tabwriter"
	"os"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/inventory"

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

func CreateOrder(orderID int, orderItems []events.OrderItem) (events.OrderCreatedPayload, error) {
	fmt.Println("==================================================================")
	fmt.Printf("                         CRIANDO PEDIDO - %d               \n", orderID)
	fmt.Println("==================================================================")

	if len(orderItems) == 0 {
		return events.OrderCreatedPayload{},fmt.Errorf("pedido sem itens")
	}

	var total float64

	for _, item := range orderItems {
		if item.Product.ID == "" {
			return events.OrderCreatedPayload{}, fmt.Errorf("pedido contém produto sem identificador")
		}

		if item.Quantity <= 0 {
			return events.OrderCreatedPayload{}, fmt.Errorf(
				"quantidade inválida para o produto %s",
				item.Product.ID,
			)
		}

		total += item.Price
	}

	order := events.OrderCreatedPayload{
		OrderID:    fmt.Sprintf("PED-%03d", orderID),
		CustomerID: "user_001",
		Items:      orderItems,
		Total:      total,
	}

	return order, nil
}

func PublishCreateOrder(channel *amqp.Channel, order events.OrderCreatedPayload) error {
	
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

	fmt.Printf(
		"[SUCESSO] Pedido %s criado com %d item(ns). Total: R$ %.2f\n",
		order.OrderID,
		len(order.Items),
		order.Total,
	)

	return nil
}

func PublishDeleteOrder(channel *amqp.Channel, orderID string) error {
	payload := events.OrderReferencePayload{
		OrderID: orderID,
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

func SelectOrderToDelete() (string, error) {
	fmt.Println("==================================================================")
	fmt.Println("                         LISTA DE PEDIDOS                       ")
	fmt.Println("==================================================================")

	var order string
		fmt.Print("Selecione um pedido para deleção: ")
		_, err := fmt.Scan(&order)
		if err != nil {
			return "", err
		}

	return order, nil
}

func ShowProducts(inventoryData inventory.Data) error {
	fmt.Println()
	fmt.Println("==================================================================")
	fmt.Println("                         LISTA DE PRODUTOS                        ")
	fmt.Println("==================================================================")

	writer := tabwriter.NewWriter(
		os.Stdout,
		0,   // largura mínima
		0,   // largura das tabulações
		3,   // espaços entre as colunas
		' ', // caractere de preench
		0,   // opções
	)

	fmt.Fprintln(
		writer,
		"ID\tPRODUTO\tCATEGORIA\tPREÇO\tESTOQUE",
	)

	fmt.Fprintln(
		writer,
		"--\t-------\t---------\t-----\t-------",
	)

	for _, product := range inventoryData.Products {
		quantity, exists := inventoryData.Stock[product.ID]

		stockStatus := "Não cadastrado"

		if exists {
			switch {
			case quantity == 0:
				stockStatus = "Esgotado"

			case quantity > 0:
				stockStatus = fmt.Sprintf("%d unidades", quantity)

			default:
				stockStatus = "Estoque inválido"
			}
		}

		fmt.Fprintf(
			writer,
			"%s\t%s\t%s\tR$ %.2f\t%s\n",
			product.ID,
			product.Name,
			product.Category,
			product.Price,
			stockStatus,
		)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf(
			"erro ao exibir produtos: %w",
			err,
		)
	}

	return nil
}

func FindProduct(
	products []events.Product,
	productID string,
) (events.Product, bool) {
	for _, product := range products {
		if product.ID == productID {
			return product, true
		}
	}

	return events.Product{}, false
}

// OrdersData representa o conteúdo do arquivo orders.json.
type OrdersData struct {
	NextOrderID int            `json:"next_order_id"`
	Orders      []events.Order `json:"orders"`
}

// LoadOrders lê e desserializa os pedidos armazenados.
func LoadOrders(filePath string) (OrdersData, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return OrdersData{}, fmt.Errorf(
			"erro ao ler arquivo de pedidos %s: %w",
			filePath,
			err,
		)
	}

	var ordersData OrdersData

	if err := json.Unmarshal(fileData, &ordersData); err != nil {
		return OrdersData{}, fmt.Errorf(
			"erro ao desserializar arquivo de pedidos %s: %w",
			filePath,
			err,
		)
	}

	// Garante que o JSON seja salvo como [] em vez de null.
	if ordersData.Orders == nil {
		ordersData.Orders = make([]events.Order, 0)
	}

	return ordersData, nil
}

// SaveOrders serializa e grava os pedidos no arquivo informado.
func SaveOrders(
	filePath string,
	ordersData OrdersData,
) error {
	if ordersData.Orders == nil {
		ordersData.Orders = make([]events.Order, 0)
	}

	fileData, err := json.MarshalIndent(
		ordersData,
		"",
		"  ",
	)
	if err != nil {
		return fmt.Errorf(
			"erro ao serializar pedidos: %w",
			err,
		)
	}

	// Acrescenta uma quebra de linha ao final do arquivo.
	fileData = append(fileData, '\n')

	if err := os.WriteFile(filePath, fileData, 0644); err != nil {
		return fmt.Errorf(
			"erro ao gravar arquivo de pedidos %s: %w",
			filePath,
			err,
		)
	}

	return nil
}

// AddOrder adiciona um novo pedido ao arquivo.
func AddOrder(
	filePath string,
	order events.Order,
	orderID int,
) error {
	if order.OrderID == "" {
		return fmt.Errorf("pedido sem identificador")
	}

	if order.Status == "" {
		order.Status = events.StatusCreated
	}

	ordersData, err := LoadOrders(filePath)
	if err != nil {
		return err
	}

	for _, registeredOrder := range ordersData.Orders {
		if registeredOrder.OrderID == order.OrderID {
			return fmt.Errorf(
				"pedido %s já está cadastrado",
				order.OrderID,
			)
		}
	}

	ordersData.Orders = append(ordersData.Orders, order)
	ordersData.NextOrderID++

	if err := SaveOrders(filePath, ordersData); err != nil {
		return fmt.Errorf(
			"erro ao salvar pedido %s: %w",
			order.OrderID,
			err,
		)
	}

	return nil
}

// UpdateOrderStatus atualiza o estado de um pedido existente.
func UpdateOrderStatus(
	filePath string,
	orderID string,
	status events.OrderStatus,
) error {
	if orderID == "" {
		return fmt.Errorf("identificador do pedido não informado")
	}

	if status == "" {
		return fmt.Errorf(
			"status não informado para o pedido %s",
			orderID,
		)
	}

	ordersData, err := LoadOrders(filePath)
	if err != nil {
		return err
	}

	for index := range ordersData.Orders {
		if ordersData.Orders[index].OrderID == orderID {
			currentStatus := ordersData.Orders[index].Status

			if currentStatus == events.StatusCancelled || currentStatus == events.StatusShipped {
				return nil
			}

			ordersData.Orders[index].Status = status

			if err := SaveOrders(filePath, ordersData); err != nil {
				return fmt.Errorf(
					"erro ao atualizar pedido %s: %w",
					orderID,
					err,
				)
			}

			return nil
		}
	}

	return fmt.Errorf("pedido %s não encontrado", orderID)
}

// ListOrders retorna todos os pedidos armazenados.
func ListOrders(
	filePath string,
) ([]events.Order, error) {
	ordersData, err := LoadOrders(filePath)
	if err != nil {
		return nil, err
	}

	orders := make(
		[]events.Order,
		len(ordersData.Orders),
	)

	copy(orders, ordersData.Orders)

	return orders, nil
}

// FindOrder procura um pedido pelo identificador.
func FindOrder(
	filePath string,
	orderID string,
) (events.Order, error) {
	if orderID == "" {
		return events.Order{}, fmt.Errorf(
			"identificador do pedido não informado",
		)
	}

	ordersData, err := LoadOrders(filePath)
	if err != nil {
		return events.Order{}, err
	}

	for _, order := range ordersData.Orders {
		if order.OrderID == orderID {
			return order, nil
		}
	}

	return events.Order{}, fmt.Errorf(
		"pedido %s não encontrado",
		orderID,
	)
}

func ShowOrders(orders []events.Order) error {
	fmt.Println()
	fmt.Println("==================================================================")
	fmt.Println("                         LISTA DE PEDIDOS                        ")
	fmt.Println("==================================================================")

	if len(orders) == 0 {
		fmt.Println("Nenhum pedido realizado.")
		fmt.Println("==================================================================")
		fmt.Println()

		return nil
	}

	writer := tabwriter.NewWriter(
		os.Stdout,
		0,
		0,
		3,
		' ',
		0,
	)

	fmt.Fprintln(
		writer,
		"ID\tSTATUS\tITENS\tTOTAL",
	)

	fmt.Fprintln(
		writer,
		"--\t------\t-----\t-----",
	)

	for _, order := range orders {
		status := string(order.Status)
		if status == "" {
			status = "NÃO INFORMADO"
		}

		fmt.Fprintf(
			writer,
			"%s\t%s\t%d\tR$ %.2f\n",
			order.OrderID,
			status,
			len(order.Items),
			order.Total,
		)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf(
			"erro ao exibir pedidos: %w",
			err,
		)
	}

	return nil
}

func HandlePrincipalEvent(
	envelope events.EventEnvelope,
	ordersFilePath string,
	channel *amqp.Channel,
) error {
	switch envelope.EventType {
	case events.PedidoEstoqueOk:
		var payload events.OrderReferencePayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pedido.estoque_ok: %w",
				err,
			)
		}

		if err := UpdateOrderStatus(
			ordersFilePath,
			payload.OrderID,
			events.StatusStockReserved,
		); err != nil {
			return fmt.Errorf(
				"erro ao atualizar o pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

	case events.EstoqueIndisponivel:
		var payload events.StockUnavailablePayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar estoque.indisponivel: %w",
				err,
			)
		}

		err := PublishDeleteOrder(channel, payload.OrderID)
			if err != nil {
				return err
		}

		if err := UpdateOrderStatus(
			ordersFilePath,
			payload.OrderID,
			events.StatusStockUnavailable,
		); err != nil {
			return fmt.Errorf(
				"erro ao atualizar o pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

	case events.PagamentoAprovado:
		var payload events.PaymentResultPayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pagamento.aprovado: %w",
				err,
			)
		}

		if err := UpdateOrderStatus(
			ordersFilePath,
			payload.OrderID,
			events.StatusPaymentApproved,
		); err != nil {
			return fmt.Errorf(
				"erro ao atualizar o pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

	case events.PagamentoRecusado:
		var payload events.PaymentResultPayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pagamento.recusado: %w",
				err,
			)
		}

		err := PublishDeleteOrder(channel, payload.OrderID)
			if err != nil {
				return err
		}

		if err := UpdateOrderStatus(
			ordersFilePath,
			payload.OrderID,
			events.StatusPaymentRefused,
		); err != nil {
			return fmt.Errorf(
				"erro ao atualizar o pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

	case events.PedidoEnviado:
		var payload events.OrderShippedPayload

		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return fmt.Errorf(
				"erro ao desserializar pedido.enviado: %w",
				err,
			)
		}

		if err := UpdateOrderStatus(
			ordersFilePath,
			payload.OrderID,
			events.StatusShipped,
		); err != nil {
			return fmt.Errorf(
				"erro ao atualizar o pedido %s: %w",
				payload.OrderID,
				err,
			)
		}

	default:
		return fmt.Errorf(
			"tipo de evento inesperado no Principal: %s",
			envelope.EventType,
		)
	}

	return nil
}