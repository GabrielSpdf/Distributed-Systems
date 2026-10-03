package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	endpoint, err := url.Parse(
		stock.baseURL + "/internal/products",
	)
	if err != nil {
		return ProductsResponse{}, fmt.Errorf(
			"erro ao interpretar endereço do estoque: %w",
			err,
		)
	}

	query := endpoint.Query()

	if category != "" {
		query.Set("category", category)
	}

	if available != "" {
		query.Set("available", available)
	}

	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		endpoint.String(),
		nil,
	)
	if err != nil {
		return ProductsResponse{}, fmt.Errorf(
			"erro ao criar requisição ao estoque: %w",
			err,
		)
	}

	request.Header.Set("Accept", "application/json")

	response, err := stock.client.Do(request)
	if err != nil {
		return ProductsResponse{}, fmt.Errorf(
			"erro ao consultar estoque: %w",
			err,
		)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return ProductsResponse{}, fmt.Errorf(
			"estoque retornou HTTP %d",
			response.StatusCode,
		)
	}

	var result ProductsResponse

	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return ProductsResponse{}, fmt.Errorf(
			"resposta inválida do estoque: %w",
			err,
		)
	}

	if result.Products == nil {
		result.Products = []Product{}
	}

	return result, nil
}
