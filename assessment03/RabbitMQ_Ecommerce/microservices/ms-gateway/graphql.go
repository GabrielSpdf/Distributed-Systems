package msgateway

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
	"RabbitMQ_Ecommerce/utils/graphqlapi"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

type graphQLResolver struct {
	stock *catalog.StockClient
}

func NewGraphQLHandler(stock *catalog.StockClient) http.Handler {
	schema := graphql.MustParseSchema(graphqlapi.CatalogSchema+`
		type Query {
			produtosDisponiveis(
				category: String
				available: Boolean = true
			): ProductList!
		}
	`, &graphQLResolver{stock: stock}, graphql.UseFieldResolvers(), graphql.MaxDepth(8))

	handler := &relay.Handler{Schema: schema}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		handler.ServeHTTP(w, r)
	})
}

func (resolver *graphQLResolver) ProdutosDisponiveis(
	ctx context.Context,
	args *graphqlapi.CatalogArgs,
) (graphqlapi.ProductList, error) {
	category := ""
	if args.Category != nil {
		category = *args.Category
	}

	available := ""
	if args.Available.Value != nil {
		available = strconv.FormatBool(*args.Available.Value)
	}

	result, err := resolver.stock.ListProducts(ctx, category, available)
	if err != nil {
		log.Printf("[ERRO] Consulta GraphQL ao Estoque: %v", err)
		return graphqlapi.ProductList{}, errors.New("catálogo temporariamente indisponível")
	}

	products := make([]graphqlapi.Product, 0, len(result.Products))
	for _, product := range result.Products {
		products = append(products, graphqlapi.Product{
			ID:       product.ID,
			Name:     product.Name,
			Category: product.Category,
			Price:    product.Price,
			Quantity: int32(product.Quantity),
		})
	}

	return graphqlapi.ProductList{
		Products: products,
		Total:    int32(result.Total),
	}, nil
}
