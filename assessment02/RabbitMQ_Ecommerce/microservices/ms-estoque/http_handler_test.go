package msestoque

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type stubCatalogStore struct {
	products   []Product
	listErr    error
	pingErr    error
	lastFilter ProductFilter
	listCalls  int
}

func (store *stubCatalogStore) ListProducts(
	_ context.Context,
	filter ProductFilter,
) ([]Product, error) {
	store.listCalls++
	store.lastFilter = filter

	return store.products, store.listErr
}

func (store *stubCatalogStore) Ping(_ context.Context) error {
	return store.pingErr
}

func serve(store CatalogStore, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()

	NewHTTPHandler(store).Routes().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, target, nil),
	)

	return recorder
}

func TestListProductsReturnsCatalog(t *testing.T) {
	store := &stubCatalogStore{
		products: []Product{
			{ID: "PROD-001", Name: "Arroz 5 kg", Category: "alimentos", Price: 29.90, Quantity: 30},
		},
	}

	recorder := serve(store, "/internal/products")

	if recorder.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200, recebeu %d: %s", recorder.Code, recorder.Body.String())
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("Content-Type inesperado: %s", contentType)
	}

	var result ProductsResponse

	if err := json.NewDecoder(recorder.Body).Decode(&result); err != nil {
		t.Fatalf("resposta JSON inválida: %v", err)
	}

	if result.Total != 1 || len(result.Products) != 1 || result.Products[0].ID != "PROD-001" {
		t.Fatalf("catálogo inesperado: %+v", result)
	}

	if store.lastFilter.Category != "" || store.lastFilter.Available != nil {
		t.Fatalf("filtro inesperado sem parâmetros: %+v", store.lastFilter)
	}
}

func TestListProductsForwardsFilters(t *testing.T) {
	store := &stubCatalogStore{products: []Product{}}

	recorder := serve(store, "/internal/products?category=%20Alimentos%20&available=true")

	if recorder.Code != http.StatusOK {
		t.Fatalf("esperava HTTP 200, recebeu %d", recorder.Code)
	}

	if store.lastFilter.Category != "alimentos" {
		t.Fatalf("categoria não normalizada: %q", store.lastFilter.Category)
	}

	if store.lastFilter.Available == nil || !*store.lastFilter.Available {
		t.Fatalf("disponibilidade não encaminhada: %+v", store.lastFilter.Available)
	}

	serve(store, "/internal/products?available=false")

	if store.lastFilter.Available == nil || *store.lastFilter.Available {
		t.Fatalf("available=false não encaminhado: %+v", store.lastFilter.Available)
	}
}

func TestListProductsReturnsEmptyArrayInsteadOfNull(t *testing.T) {
	recorder := serve(&stubCatalogStore{products: []Product{}}, "/internal/products")

	if !strings.Contains(recorder.Body.String(), `"products":[]`) {
		t.Fatalf("esperava lista vazia, recebeu %s", recorder.Body.String())
	}
}

func TestListProductsRejectsInvalidAvailability(t *testing.T) {
	store := &stubCatalogStore{}

	recorder := serve(store, "/internal/products?available=talvez")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("esperava HTTP 400, recebeu %d", recorder.Code)
	}

	var body map[string]string

	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("resposta JSON inválida: %v", err)
	}

	if body["code"] != "INVALID_REQUEST" || body["message"] == "" {
		t.Fatalf("corpo de erro inesperado: %+v", body)
	}

	if store.listCalls != 0 {
		t.Fatal("o banco foi consultado com filtro inválido")
	}
}

func TestListProductsReportsStoreFailure(t *testing.T) {
	recorder := serve(
		&stubCatalogStore{listErr: errors.New("banco fora do ar")},
		"/internal/products",
	)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("esperava HTTP 500, recebeu %d", recorder.Code)
	}

	if strings.Contains(recorder.Body.String(), "banco fora do ar") {
		t.Fatal("detalhe interno do erro foi exposto na resposta")
	}

	if !strings.Contains(recorder.Body.String(), `"code":"INTERNAL_ERROR"`) {
		t.Fatalf("corpo de erro inesperado: %s", recorder.Body.String())
	}
}

func TestHealthReflectsDatabase(t *testing.T) {
	healthy := serve(&stubCatalogStore{}, "/healthz")

	if healthy.Code != http.StatusOK || !strings.Contains(healthy.Body.String(), `"status":"healthy"`) {
		t.Fatalf("resposta saudável inesperada: %d %s", healthy.Code, healthy.Body.String())
	}

	unhealthy := serve(&stubCatalogStore{pingErr: errors.New("sem conexão")}, "/healthz")

	if unhealthy.Code != http.StatusServiceUnavailable ||
		!strings.Contains(unhealthy.Body.String(), `"status":"unhealthy"`) {
		t.Fatalf("resposta indisponível inesperada: %d %s", unhealthy.Code, unhealthy.Body.String())
	}
}
