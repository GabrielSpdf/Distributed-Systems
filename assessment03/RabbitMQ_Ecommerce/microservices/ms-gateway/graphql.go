package msgateway

import (
	"context"
	"errors"
	"log"
	"mime"
	"net/http"
	"strconv"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
	"RabbitMQ_Ecommerce/microservices/ms-gateway/catalog"
	"RabbitMQ_Ecommerce/microservices/ms-gateway/orders"
	"RabbitMQ_Ecommerce/utils/graphqlapi"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

type graphQLRequestKey struct{}

type graphQLResolver struct {
	stock           *catalog.StockClient
	orderRepository *orders.Repository
	authentication  *auth.HTTPHandler
}

func NewGraphQLHandler(
	stock *catalog.StockClient,
	orderRepository *orders.Repository,
	authentication *auth.HTTPHandler,
) http.Handler {
	schema := graphql.MustParseSchema(graphqlapi.CatalogSchema+`
		type Query {
			produtosDisponiveis(
				category: String
				available: Boolean = true
			): ProductList!
		}
		type Mutation {
			criarPedido(items: [OrderItemInput!]!): CreatedOrder
		}

		input OrderItemInput {
			productId: String!
			quantity: Int!
		}

		type CreatedOrder {
			id: String!
			status: String!
		}
	`, &graphQLResolver{stock: stock, orderRepository: orderRepository, authentication: authentication}, graphql.UseFieldResolvers(), graphql.MaxDepth(8))

	handler := &relay.Handler{Schema: schema}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "use Content-Type: application/json", http.StatusUnsupportedMediaType)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		ctx := context.WithValue(r.Context(), graphQLRequestKey{}, r)
		handler.ServeHTTP(w, r.WithContext(ctx))
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
