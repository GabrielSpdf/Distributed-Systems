package main

import (
	"encoding/json"
	"log"
	"net/http"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
)

var products = []catalog.Product{
	{ID: "PROD-001", Name: "Arroz 5 kg", Category: "alimentos", Price: 29.90, Quantity: 30},
	{ID: "PROD-002", Name: "Detergente", Category: "limpeza", Price: 4.90, Quantity: 50},
	{ID: "PROD-003", Name: "Fone Bluetooth", Category: "eletronicos", Price: 119.90, Quantity: 12},
	{ID: "PROD-004", Name: "Feijão 1 kg", Category: "alimentos", Price: 8.90, Quantity: 25},
	{ID: "PROD-005", Name: "Café 500 g", Category: "alimentos", Price: 24.90, Quantity: 18},
	{ID: "PROD-006", Name: "Açúcar 1 kg", Category: "alimentos", Price: 4.50, Quantity: 40},
	{ID: "PROD-007", Name: "Macarrão 500 g", Category: "alimentos", Price: 5.90, Quantity: 35},
	{ID: "PROD-008", Name: "Leite 1 L", Category: "alimentos", Price: 6.20, Quantity: 20},
	{ID: "PROD-009", Name: "Sabão em pó 1 kg", Category: "limpeza", Price: 14.90, Quantity: 15},
	{ID: "PROD-010", Name: "Desinfetante 2 L", Category: "limpeza", Price: 9.90, Quantity: 22},
	{ID: "PROD-011", Name: "Água sanitária 1 L", Category: "limpeza", Price: 4.20, Quantity: 28},
	{ID: "PROD-012", Name: "Mouse sem fio", Category: "eletronicos", Price: 59.90, Quantity: 10},
	{ID: "PROD-013", Name: "Teclado USB", Category: "eletronicos", Price: 79.90, Quantity: 8},
	{ID: "PROD-014", Name: "Carregador USB-C", Category: "eletronicos", Price: 49.90, Quantity: 0},
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /internal/products", listProducts)

	log.Println("[MOCK] Estoque simulado em http://localhost:8081")
	log.Fatal(http.ListenAndServe("127.0.0.1:8081", mux))
}

func listProducts(
	writer http.ResponseWriter,
	request *http.Request,
) {
	category := request.URL.Query().Get("category")
	available := request.URL.Query().Get("available")

	result := make([]catalog.Product, 0)

	for _, product := range products {
		if category != "" && product.Category != category {
			continue
		}

		if available == "true" && product.Quantity <= 0 {
			continue
		}

		if available == "false" && product.Quantity > 0 {
			continue
		}

		result = append(result, product)
	}

	writer.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(writer).Encode(
		catalog.ProductsResponse{
			Products: result,
			Total:    len(result),
		},
	); err != nil {
		log.Printf("[ERRO] Resposta do mock: %v", err)
	}
}
