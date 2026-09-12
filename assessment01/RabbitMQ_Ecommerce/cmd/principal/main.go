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
		log.Println("[ERRO] Erro ao executar o sistema:", err)
		log.Fatal(err)
	}
}

func run() error {
	msPrincipal, err := msprincipal.InitMSPrincipal()
	if err != nil {
		return err
	}

	fmt.Print("[SUCESSO] Microsserviço principal inicializado com sucesso")
	time.Sleep(3 * time.Second)
	fmt.Print("\033[H\033[2J")

	defer msPrincipal.Connection.Close()
	defer msPrincipal.Channel.Close()

	consumerErrors := make(chan error, 1)

	go func() {
		err := rabbitmq.ConsumeEvents(
			msPrincipal.Channel,
			msPrincipal.QueueName,
			func(envelope events.EventEnvelope) error {
				return msprincipal.HandlePrincipalEvent(
					envelope,
					"data/orders.json",
					msPrincipal.Channel,
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
		fmt.Println("3. Excluir pedido")
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

		ordersData, err := msprincipal.LoadOrders("data/orders.json")
		if err != nil {
			return err
		}

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

			orderPayload,err := msprincipal.PublishCreateOrder(msPrincipal.Channel, orderID, orderItems)
			if err != nil {
				return err
			}

			order = events.Order{
				OrderID:    orderPayload.OrderID,
				CustomerID: orderPayload.CustomerID,
				Items:      orderPayload.Items,
				Total:      orderPayload.Total,
Status:     events.StatusCreated,
			}

			if err := msprincipal.AddOrder("data/orders.json", order, orderID); err != nil {
				return fmt.Errorf(
					"erro ao salvar pedido %s: %w",
					order.OrderID,
					err,
				)
			}

		case 3:
			// Excluir pedido
			if len(ordersData.Orders) == 0 {
				fmt.Println("==================================================================")
				fmt.Println("Nenhum pedido cadastrado para exclusão.")
				continue
			}

			if err := msprincipal.ShowOrders(ordersData.Orders); err != nil {
				return err
			}

			orderID, err := msprincipal.SelectOrderToDelete()
			if err != nil {
				return err
			}

			if _, err := msprincipal.FindOrder("data/orders.json", orderID); err != nil {
				fmt.Printf("[ERRO] %s\n", err)
				continue
			}

			err = msprincipal.PublishDeleteOrder(msPrincipal.Channel, orderID)
			if err != nil {
				return err
			}

			err = msprincipal.UpdateOrderStatus("data/orders.json", orderID, events.StatusCancelled)
			if err != nil {
				return fmt.Errorf(
					"erro ao atualizar status do pedido %s: %w",
					orderID,
					err,
				)
			}

			fmt.Printf("[SUCESSO] Pedido %s excluído com sucesso\n", orderID)
		case 4:
			// Consultar pedidos realizados
			if err := msprincipal.ShowOrders(ordersData.Orders); err != nil {
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
