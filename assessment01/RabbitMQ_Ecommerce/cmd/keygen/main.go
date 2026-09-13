package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"RabbitMQ_Ecommerce/utils/cryptography"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal(
			"uso: go run ./cmd/keygen <nome-do-microsserviço>",
		)
	}

	serviceName := os.Args[1]

	allowedServices := map[string]bool{
		"ms-principal": true,
		"ms-estoque":   true,
		"ms-pagamento": true,
		"ms-entrega":   true,
	}

	if !allowedServices[serviceName] {
		log.Fatalf(
			"microsserviço desconhecido: %s",
			serviceName,
		)
	}

	keyDirectory := filepath.Join(
		"keys",
		serviceName,
	)

	if err := cryptography.GenerateKeyPair(
		keyDirectory,
	); err != nil {
		log.Fatal(err)
	}

	fmt.Printf(
		"[SUCESSO] Chaves do %s geradas em %s\n",
		serviceName,
		keyDirectory,
	)
}
