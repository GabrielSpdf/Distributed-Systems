package msgateway

import (
	"context"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/orders"
)

func (resolver *graphQLResolver) PedidoAtualizado(
	ctx context.Context,
) (<-chan orders.OrderStatusUpdate, error) {
	userID, err := resolver.authenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}

	return resolver.orderRepository.SubscribeOrderStatus(ctx, userID), nil
}
