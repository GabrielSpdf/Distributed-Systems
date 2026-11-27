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
	"RabbitMQ_Ecommerce/microservices/ms-gateway/promotions"
	"RabbitMQ_Ecommerce/utils/graphqlapi"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
	graphqlws "github.com/graph-gophers/graphql-transport-ws"
)

type graphQLRequestKey struct{}

type graphQLWriterKey struct{}

type graphQLResolver struct {
	stock               *catalog.StockClient
	orderRepository     *orders.Repository
	authentication      *auth.HTTPHandler
	promotionRepository *promotions.Repository
}

func NewGraphQLHandler(
	stock *catalog.StockClient,
	orderRepository *orders.Repository,
	authentication *auth.HTTPHandler,
	promotionRepository *promotions.Repository,
	frontendOrigin string,
) http.Handler {
	schema := graphql.MustParseSchema(graphqlapi.CatalogSchema+`
		type Query {
			produtosDisponiveis(
				category: String
				available: Boolean = true
			): ProductList!
			pedidos: OrderList
			pedido(id: String!): Order
			interessesPromocao: PromotionInterests!
			usuarioAtual: User
		}
		type Mutation {
			criarPedido(items: [OrderItemInput!]!): CreatedOrder
			cancelarPedido(id: String!): Boolean!
			registrarInteressePromocao(categories: [String!]!, email: String): PromotionInterestChange!
			cancelarInteressePromocao(category: String!): PromotionInterestChange!
			cadastrarUsuario(name: String!, email: String!, password: String!): User
			entrar(email: String!, password: String!): User
			sair: Boolean!
		}

		input OrderItemInput {
			productId: String!
			quantity: Int!
		}

		type CreatedOrder {
			id: String!
			status: String!
		}
		type OrderList {
			orders: [Order!]!
			total: Int!
		}
		type Order {
			id: String!
			status: String!
			items: [OrderItem!]!
			total: Float!
			currency: String!
			canCancel: Boolean!
			checkoutUrl: String
			checkoutExpiresAt: String
			invoiceId: String
			trackingCode: String
			shippedAt: String
			createdAt: String!
			updatedAt: String!
		}
		type OrderItem {
			productId: String!
			productName: String!
			quantity: Int!
			unitPrice: Float!
		}
		type PromotionInterests {
			categories: [String!]!
			email: String!
		}

		type PromotionInterestChange {
			changed: Boolean!
		}
		type User {
			id: String!
			name: String!
			email: String!
			createdAt: String!
		}
		type Subscription {
			pedidoAtualizado: OrderStatusUpdate!
		}
		type OrderStatusUpdate {
			eventId: String!
			orderId: String!
			status: String!
		}
	`, &graphQLResolver{stock: stock, orderRepository: orderRepository, authentication: authentication, promotionRepository: promotionRepository}, graphql.UseFieldResolvers(), graphql.MaxDepth(8))

	handler := &relay.Handler{Schema: schema}
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "use POST para operações GraphQL via HTTP", http.StatusMethodNotAllowed)
			return
		}

		mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mediaType != "application/json" {
			http.Error(w, "use Content-Type: application/json", http.StatusUnsupportedMediaType)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
		ctx := context.WithValue(r.Context(), graphQLRequestKey{}, r)
		ctx = context.WithValue(ctx, graphQLWriterKey{}, w)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})

	return graphqlws.NewHandlerFunc(
		schema,
		httpHandler,
		graphqlws.WithCheckOrigin(func(r *http.Request) bool {
			return frontendOrigin != "" && r.Header.Get("Origin") == frontendOrigin
		}),
		graphqlws.WithContextGenerator(graphqlws.ContextGeneratorFunc(
			func(ctx context.Context, r *http.Request) (context.Context, error) {
				return context.WithValue(ctx, graphQLRequestKey{}, r), nil
			},
		)),
		graphqlws.WithMaxSubscriptions(4),
		graphqlws.WithReadLimit(64*1024),
		graphqlws.WithReadIdleTimeout(0),
	)
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
