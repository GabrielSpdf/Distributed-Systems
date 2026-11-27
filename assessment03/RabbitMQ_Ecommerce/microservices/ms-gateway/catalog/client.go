package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Product struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

type ProductsResponse struct {
	Products []Product `json:"products"`
	Total    int       `json:"total"`
}

type StockClient struct {
	baseURL string
	client  *http.Client
}

func NewStockClient(baseURL string) *StockClient {
	return &StockClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (stock *StockClient) ListProducts(
	ctx context.Context,
	category string,
	available string,
) (ProductsResponse, error) {
	const query = `
		query Catalog($category: String, $available: Boolean) {
			produtosDisponiveis(category: $category, available: $available) {
				products { id name category price quantity }
				total
			}
		}
	`

	var availability any
	switch available {
	case "":
		availability = nil
	case "true":
		availability = true
	case "false":
		availability = false
	default:
		return ProductsResponse{}, fmt.Errorf("available deve ser true ou false")
	}

	body, err := json.Marshal(struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}{
		Query: query,
		Variables: map[string]any{
			"category":  category,
			"available": availability,
		},
	})
	if err != nil {
		return ProductsResponse{}, fmt.Errorf("erro ao preparar consulta ao estoque: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		stock.baseURL+"/graphql",
		bytes.NewReader(body),
	)
	if err != nil {
		return ProductsResponse{}, fmt.Errorf("erro ao criar requisição ao estoque: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")

	response, err := stock.client.Do(request)
	if err != nil {
		return ProductsResponse{}, fmt.Errorf("erro ao consultar estoque: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return ProductsResponse{}, fmt.Errorf("estoque retornou HTTP %d", response.StatusCode)
	}

	var result struct {
		Data struct {
			Catalog *ProductsResponse `json:"produtosDisponiveis"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return ProductsResponse{}, fmt.Errorf("resposta inválida do estoque: %w", err)
	}
	if len(result.Errors) > 0 {
		return ProductsResponse{}, fmt.Errorf("GraphQL do estoque: %s", result.Errors[0].Message)
	}
	if result.Data.Catalog == nil || result.Data.Catalog.Products == nil {
		return ProductsResponse{}, fmt.Errorf("estoque retornou catálogo incompleto")
	}

	return *result.Data.Catalog, nil
}
