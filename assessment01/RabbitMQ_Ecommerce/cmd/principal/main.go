package main

import (
	"log"
	"fmt"
	"time"

	"RabbitMQ_Ecommerce/microsservices/ms-principal"
)

func main() {
	if err := run(); err != nil {
		log.Println("[ERRO] Erro ao executar o sistema:", err)
		log.Fatal(err)
	}
}

func run() error {
	var orderID int = 1

	msPrincipal, err := msprincipal.InitMSPrincipal()
	if err != nil {
		return err
	}

	fmt.Print("[SUCESSO] Microsserviço principal inicializado com sucesso")
	time.Sleep(3 * time.Second)
	fmt.Print("\033[H\033[2J")

	defer msPrincipal.Connection.Close()
	defer msPrincipal.Channel.Close()

	fmt.Println("==================================================================")
	fmt.Println("          BEM-VINDO AO SISTEMA DISTRIBUÍDO DE E-COMMERCE          ")

	var exit bool 

	for !exit {
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

		switch option {
		case 1:
			// Visualizar produtos
		case 2:
			// Realizar pedido
			err := msprincipal.PublishCreateOrder(msPrincipal.Channel, orderID)
			if err != nil {
				return err
			}
			orderID += 1

		case 3:
			// Excluir pedido
			order, err := msprincipal.ShowOrders()
			if err != nil {
				return err
			}

			err = msprincipal.PublishDeleteOrder(msPrincipal.Channel, order)
			if err != nil {
				return err
			}
		case 4:
			// Consultar pedidos realizados
		case 5:
			// Sair
			exit = true
		default:
			fmt.Println("[ERRO] Opção inválida! Selecione uma opção válida.")
		}
	}

	return nil
}
