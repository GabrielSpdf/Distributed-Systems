package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"

	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

// Devolve à fila original as mensagens paradas em "<fila>.revisao", depois que
// a causa da falha (banco, Mock, RabbitMQ) foi resolvida.
//
// Uso: go run ./cmd/reprocessar -fila pagamento [-limite 100]
func main() {
	queue := flag.String("fila", "", "fila original: estoque, pagamento ou entrega")
	limit := flag.Int("limite", 100, "quantidade máxima de mensagens a devolver")
	flag.Parse()

	if strings.TrimSpace(*queue) == "" || *limit <= 0 {
		log.Fatal("uso: go run ./cmd/reprocessar -fila <estoque|pagamento|entrega> [-limite N]")
	}

	connection, err := rabbitmq.Connect()
	if err != nil {
		log.Fatalf("[ERRO] %v", err)
	}
	defer connection.Close()

	moved, err := rabbitmq.ReprocessReview(context.Background(), connection, *queue, *limit)
	if err != nil {
		log.Fatalf("[ERRO] %d mensagem(ns) devolvida(s) antes da falha: %v", moved, err)
	}

	fmt.Printf("[SUCESSO] %d mensagem(ns) devolvida(s) de %s para %s\n", moved, rabbitmq.ReviewQueueName(*queue), *queue)
}
