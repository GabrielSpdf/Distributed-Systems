package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListProducts(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			if request.Method != http.MethodGet {
				t.Errorf("esperava GET, recebeu %s", request.Method)
			}

			if request.URL.Path != "/internal/products" {
				t.Errorf("caminho inesperado: %s", request.URL.Path)
			}

			if request.URL.Query().Get("category") != "alimentos" {
				t.Error("filtro de categoria não foi enviado")
			}

			if request.URL.Query().Get("available") != "true" {
				t.Error("filtro de disponibilidade não foi enviado")
			}

			writer.Header().Set("Content-Type", "application/json")

			_ = json.NewEncoder(writer).Encode(ProductsResponse{
				Products: []Product{
					{
						ID:       "PROD-001",
						Name:     "Arroz 5 kg",
						Category: "alimentos",
						Price:    29.90,
						Quantity: 30,
					},
				},
				Total: 1,
			})
		}),
	)
	defer server.Close()

	client := NewStockClient(server.URL)

	result, err := client.ListProducts(
		context.Background(),
		"alimentos",
		"true",
	)
	if err != nil {
		t.Fatalf("não esperava erro: %v", err)
	}

	if result.Total != 1 || len(result.Products) != 1 {
		t.Fatalf("esperava um produto, recebeu %+v", result)
	}

	if result.Products[0].ID != "PROD-001" {
		t.Fatalf("produto inesperado: %+v", result.Products[0])
	}
}

func TestListProductsReturnsErrorWhenStockFails(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			_ *http.Request,
		) {
			writer.WriteHeader(http.StatusServiceUnavailable)
		}),
	)
	defer server.Close()

	client := NewStockClient(server.URL)

	_, err := client.ListProducts(
		context.Background(),
		"",
		"",
	)
	if err == nil {
		t.Fatal("esperava erro quando o estoque retorna HTTP 503")
	}
}
