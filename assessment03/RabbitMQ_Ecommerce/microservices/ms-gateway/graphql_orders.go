package msgateway

import (
	"context"
	"errors"
	"log"
	"net/http"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/orders"
)

type createOrderItemInput struct {
	ProductID string
	Quantity  int32
}

type createdOrder struct {
	ID     string
	Status string
}

func (resolver *graphQLResolver) CriarPedido(
	ctx context.Context,
	args *struct{ Items []createOrderItemInput },
) (*createdOrder, error) {
	request, ok := ctx.Value(graphQLRequestKey{}).(*http.Request)
	if !ok {
		return nil, errors.New("requisição indisponível")
	}

	userID, status, message := resolver.authentication.AuthenticateRequest(request.WithContext(ctx))
	if status != http.StatusOK {
		return nil, errors.New(message)
	}

	catalog, err := resolver.stock.ListProducts(ctx, "", "")
	if err != nil {
		log.Printf("[ERRO] Consulta ao estoque: %v", err)
		return nil, errors.New("não foi possível consultar os produtos")
	}

	requested := make([]orders.CreateOrderItem, 0, len(args.Items))
	for _, item := range args.Items {
		requested = append(requested, orders.CreateOrderItem{
			ProductID: item.ProductID,
			Quantity:  int(item.Quantity),
		})
	}

	items, totalCents, err := orders.PrepareItems(requested, catalog.Products)
	if err != nil {
		return nil, err
	}

	order, err := resolver.orderRepository.Create(ctx, userID, items, totalCents)
	if err != nil {
		log.Printf("[ERRO] Criação do pedido: %v", err)
		return nil, errors.New("não foi possível salvar o pedido")
	}

	log.Printf("[INFO] Pedido %s e evento de criação salvos na outbox", order.ID)
	return &createdOrder{ID: order.ID, Status: order.Status}, nil
}
