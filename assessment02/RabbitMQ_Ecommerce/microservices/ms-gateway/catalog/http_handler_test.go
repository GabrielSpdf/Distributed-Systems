package catalog

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProductsRouteReturnsCatalog(t *testing.T) {
	stockServer := httptest.NewServer(
		http.HandlerFunc(func(
			writer http.ResponseWriter,
			request *http.Request,
		) {
			if request.URL.Path != "/internal/products" {
				t.Errorf("caminho inesperado: %s", request.URL.Path)
			}

			if request.URL.Query().Get("category") != "alimentos" {
				t.Error("categoria não foi encaminhada ao estoque")
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
	defer stockServer.Close()

	handler := NewHTTPHandler(
		NewStockClient(stockServer.URL),
	)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/products?category=alimentos",
		nil,
	)
	recorder := httptest.NewRecorder()

	handler.Routes().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf(
			"esperava HTTP 200, recebeu %d: %s",
			recorder.Code,
			recorder.Body.String(),
		)
	}

	var result ProductsResponse

	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("resposta JSON inválida: %v", err)
	}

	if result.Total != 1 || len(result.Products) != 1 {
		t.Fatalf("esperava um produto, recebeu %+v", result)
	}

	if result.Products[0].Name != "Arroz 5 kg" {
		t.Fatalf("produto inesperado: %+v", result.Products[0])
	}
}
