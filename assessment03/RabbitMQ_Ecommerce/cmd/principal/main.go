package main

import (
	"fmt"
	"log"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-principal"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/inventory"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf(
			"[ERRO] Erro ao executar o sistema: %v",
			err,
		)
	}
}

func run() error {
	principalService, err := msprincipal.InitializePrincipalService()
	if err != nil {
		return err
	}

	fmt.Print("[SUCESSO] Microsserviço principal inicializado com sucesso")
	time.Sleep(3 * time.Second)
	fmt.Print("\033[H\033[2J")

	defer principalService.Connection.Close()
	defer principalService.PublisherChannel.Close()
	defer principalService.ConsumerChannel.Close()

	consumerErrors := make(chan error, 1)

	ordersRepository := msprincipal.NewOrderRepository(
		"data/orders.json",
	)

	go func() {
		err := rabbitmq.ConsumeSignedEvents(
			principalService.ConsumerChannel,
			principalService.QueueName,
			principalService.PublicKeys,
			func(envelope events.EventEnvelope) error {
				return msprincipal.HandlePrincipalEvent(
					envelope,
					ordersRepository,
					principalService.ConsumerChannel,
					principalService.PrivateKey,
				)
			},
		)

		consumerErrors <- err
	}()

	fmt.Println("==================================================================")
	fmt.Println("          BEM-VINDO AO SISTEMA DISTRIBUÍDO DE E-COMMERCE          ")

	var exit bool

	for !exit {
		select {
		case err := <-consumerErrors:
			return fmt.Errorf("consumidor de eventos encerrado: %w", err)
		default:
		}

		fmt.Println("==================================================================")
		fmt.Println("                         MENU PRINCIPAL                           ")
		fmt.Println("==================================================================")
		fmt.Println("1. Visualizar produtos")
		fmt.Println("2. Realizar pedido")
		fmt.Println("3. Excluir pedido (Remover da visualização)")
		fmt.Println("4. Consultar pedidos realizados")
		fmt.Println("5. Sair")

		var option int
		fmt.Print("Escolha uma opção: ")
		_, err := fmt.Scan(&option)
		if err != nil {
			return err
		}

		inventoryData, err := inventory.Load("inventory.json")
		if err != nil {
			return err
		}

		ordersData, err := ordersRepository.Load()
		if err != nil {
			return err
		}

		visibleOrders := msprincipal.FilterVisibleOrders(
			ordersData.Orders,
		)

		orderID := ordersData.NextOrderID

		switch option {
		case 1:
			// Visualizar produtos
			if err := msprincipal.ShowProducts(inventoryData); err != nil {
				return err
			}
		case 2:
			// Realizar pedido
			fmt.Println("==================================================================")
			fmt.Printf("                         REALIZANDO PEDIDO - %d               \n", orderID)
			fmt.Println("==================================================================")

			orderItems := make([]events.OrderItem, 0)
			var order events.Order

			for {
				var productID string
				var quantity int

				fmt.Print("Digite o ID do produto: ")
				if _, err := fmt.Scan(&productID); err != nil {
					return fmt.Errorf("erro ao ler ID do produto: %w", err)
				}

				product, exists := msprincipal.FindProduct(
					inventoryData.Products,
					productID,
				)
				if !exists {
					fmt.Printf("[ERRO] Produto %s não encontrado\n", productID)
					continue
				}

				fmt.Printf(
					"Produto selecionado: %s — R$ %.2f\n",
					product.Name,
					product.Price,
				)

				fmt.Print("Digite a quantidade: ")
				if _, err := fmt.Scan(&quantity); err != nil {
					return fmt.Errorf("erro ao ler quantidade: %w", err)
				}

				if quantity <= 0 {
					fmt.Println("[ERRO] A quantidade deve ser maior que zero")
					continue
				}

				orderItem := events.OrderItem{
					Product:  product,
					Quantity: quantity,
					Price:    product.Price * float64(quantity),
				}

				orderItems = append(orderItems, orderItem)

				fmt.Printf(
					"[OK] %d unidade(s) de %s adicionada(s): R$ %.2f\n",
					quantity,
					product.Name,
					orderItem.Price,
				)

				var continueOrder string

				fmt.Print("Deseja adicionar outro item ao pedido? (s/n): ")
				if _, err := fmt.Scan(&continueOrder); err != nil {
					return fmt.Errorf(
						"erro ao ler confirmação do pedido: %w",
						err,
					)
				}

				if continueOrder != "s" && continueOrder != "S" {
					break
				}
			}

			orderPayload, err := msprincipal.CreateOrder(orderID, orderItems)
			if err != nil {
				return fmt.Errorf("erro ao criar pedido: %w", err)
			}

			order = events.Order{
				OrderID:    orderPayload.OrderID,
				CustomerID: orderPayload.CustomerID,
				Items:      orderPayload.Items,
				Total:      orderPayload.Total,
				Status:     events.StatusPending,
			}

			if err := ordersRepository.Add(order); err != nil {
				return fmt.Errorf(
					"erro ao salvar pedido %s: %w",
					order.OrderID,
					err,
				)
			}

			err = msprincipal.PublishOrderCreated(principalService.PublisherChannel, principalService.PrivateKey, orderPayload)
			if err != nil {
				updateStatusErr := ordersRepository.UpdateStatus(orderPayload.OrderID, events.StatusProcessingFailed)
				if updateStatusErr != nil {
					return fmt.Errorf(
						"erro ao publicar pedido %s: %w e erro ao atualizar status do pedido: %v",
						orderPayload.OrderID,
						err,
						updateStatusErr,
					)
				}
				return fmt.Errorf(
					"pedido %s registrado como %s após falha na publicação: %w",
					orderPayload.OrderID,
					events.StatusProcessingFailed,
					err,
				)
			}

		case 3:
			// Excluir pedido
			if len(visibleOrders) == 0 {
				fmt.Println("==================================================================")
				fmt.Println("Nenhum pedido disponível para exclusão.")
				continue
			}

			if err := msprincipal.ShowOrders(visibleOrders); err != nil {
				return err
			}

			orderID, err := msprincipal.SelectOrderToHide()
			if err != nil {
				return err
			}

			order, err := ordersRepository.Find(orderID)
			if err != nil {
				fmt.Printf("[ERRO] %s\n", err)
				continue
			}

			if order.IsDeleted {
				fmt.Printf(
					"[ERRO] Pedido %s já foi removido da visualização\n",
					orderID,
				)
				continue
			}

			if err := ordersRepository.MarkAsDeleted(orderID); err != nil {
				return fmt.Errorf(
					"erro ao excluir logicamente o pedido %s: %w",
					orderID,
					err,
				)
			}

			fmt.Printf(
				"[SUCESSO] Pedido %s removido da visualização\n",
				orderID,
			)
		case 4:
			// Consultar pedidos realizados
			if err := msprincipal.ShowOrders(visibleOrders); err != nil {
				return err
			}

		case 5:
			// Sair
			exit = true
		default:
			fmt.Println("[ERRO] Opção inválida! Selecione uma opção válida.")
		}
	}

	return nil
}
