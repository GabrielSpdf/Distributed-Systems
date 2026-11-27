package msgateway

import (
	"context"
	"log"
	"slices"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/orders"
	"RabbitMQ_Ecommerce/microservices/ms-gateway/promotions"
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

func (resolver *graphQLResolver) PromocaoDisponivel(
	ctx context.Context,
) (<-chan promotions.Offer, error) {
	userID, err := resolver.authenticatedUserID(ctx)
	if err != nil {
		return nil, err
	}

	offers, unsubscribe := resolver.promotionOffers.Subscribe()
	updates := make(chan promotions.Offer)

	go func() {
		defer close(updates)
		defer unsubscribe()

		for {
			select {
			case <-ctx.Done():
				return
			case offer := <-offers:
				lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				categories, err := resolver.promotionRepository.ListActive(lookupCtx, userID)
				cancel()

				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Printf("[AVISO] Falha ao consultar interesses para a oferta %s: %v", offer.EventID, err)
					continue
				}
				if !slices.Contains(categories, offer.Category) {
					continue
				}

				select {
				case updates <- offer:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return updates, nil
}
