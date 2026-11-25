package msestoque

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"RabbitMQ_Ecommerce/utils/graphqlapi"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

type catalogResolver struct {
	store CatalogStore
}

func NewGraphQLHandler(store CatalogStore) http.Handler {
	schema := graphql.MustParseSchema(graphqlapi.CatalogSchema+`
		type Query {
			produtosDisponiveis(
				category: String
				available: Boolean = true
			): ProductList!
		}
	`, &catalogResolver{store: store}, graphql.UseFieldResolvers(), graphql.MaxDepth(8))

	handler := &relay.Handler{Schema: schema}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		handler.ServeHTTP(w, r)
	})
}

func (resolver *catalogResolver) ProdutosDisponiveis(
	ctx context.Context,
	args *graphqlapi.CatalogArgs,
) (graphqlapi.ProductList, error) {
	filter := ProductFilter{Available: args.Available.Value}
	if args.Category != nil {
		filter.Category = strings.ToLower(strings.TrimSpace(*args.Category))
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	products, err := resolver.store.ListProducts(ctx, filter)
	if err != nil {
		log.Printf("[ERRO] Consulta GraphQL de produtos: %v", err)
		return graphqlapi.ProductList{}, errors.New("não foi possível consultar os produtos")
	}

	result := graphqlapi.ProductList{
		Products: make([]graphqlapi.Product, 0, len(products)),
		Total:    int32(len(products)),
	}

	for _, product := range products {
		result.Products = append(result.Products, graphqlapi.Product{
			ID:       product.ID,
			Name:     product.Name,
			Category: product.Category,
			Price:    product.Price,
			Quantity: int32(product.Quantity),
		})
	}

	return result, nil
}
