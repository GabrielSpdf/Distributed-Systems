package msgateway

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

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
	userID, err := resolver.orderUserID(ctx)
	if err != nil {
		return nil, err
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

func (resolver *graphQLResolver) orderUserID(ctx context.Context) (string, error) {
	request, ok := ctx.Value(graphQLRequestKey{}).(*http.Request)
	if !ok {
		return "", errors.New("requisição indisponível")
	}

	userID, status, message := resolver.authentication.AuthenticateRequest(request.WithContext(ctx))
	if status != http.StatusOK {
		return "", errors.New(message)
	}
	return userID, nil
}

type orderListResult struct {
	Orders []orderResult
	Total  int32
}

type orderResult struct {
	ID                string
	Status            string
	Items             []orderItemResult
	Total             float64
	Currency          string
	CanCancel         bool
	CheckoutURL       *string
	CheckoutExpiresAt *string
	InvoiceID         *string
	TrackingCode      *string
	ShippedAt         *string
	CreatedAt         string
	UpdatedAt         string
}

type orderItemResult struct {
	ProductID   string
	ProductName string
	Quantity    int32
	UnitPrice   float64
}

func (resolver *graphQLResolver) Pedidos(ctx context.Context) (*orderListResult, error) {
	userID, err := resolver.orderUserID(ctx)
	if err != nil {
		return nil, err
	}

	records, err := resolver.orderRepository.ListByUser(ctx, userID)
	if err != nil {
		log.Printf("[ERRO] Listagem de pedidos: %v", err)
		return nil, errors.New("não foi possível consultar os pedidos")
	}

	result := &orderListResult{
		Orders: make([]orderResult, 0, len(records)),
		Total:  int32(len(records)),
	}
	now := time.Now()
	for _, order := range records {
		result.Orders = append(result.Orders, toOrderResult(order, now))
	}
	return result, nil
}

func (resolver *graphQLResolver) Pedido(
	ctx context.Context,
	args *struct{ ID string },
) (*orderResult, error) {
	userID, err := resolver.orderUserID(ctx)
	if err != nil {
		return nil, err
	}

	order, err := resolver.orderRepository.FindByID(ctx, userID, args.ID)
	if errors.Is(err, orders.ErrOrderNotFound) {
		return nil, errors.New("pedido não encontrado")
	}
	if err != nil {
		log.Printf("[ERRO] Consulta do pedido: %v", err)
		return nil, errors.New("não foi possível consultar o pedido")
	}

	result := toOrderResult(order, time.Now())
	return &result, nil
}

func (resolver *graphQLResolver) CancelarPedido(
	ctx context.Context,
	args *struct{ ID string },
) (bool, error) {
	userID, err := resolver.orderUserID(ctx)
	if err != nil {
		return false, err
	}

	_, err = resolver.orderRepository.Cancel(ctx, userID, args.ID)
	switch {
	case errors.Is(err, orders.ErrOrderNotFound):
		return false, errors.New("pedido não encontrado")
	case errors.Is(err, orders.ErrCancellationConflict):
		return false, errors.New("o processamento já começou; não é mais possível cancelar este pedido")
	case err != nil:
		log.Printf("[ERRO] Cancelamento do pedido: %v", err)
		return false, errors.New("não foi possível cancelar o pedido")
	}
	return true, nil
}

func toOrderResult(order orders.Order, now time.Time) orderResult {
	result := orderResult{
		ID:           order.ID,
		Status:       order.Status,
		Items:        make([]orderItemResult, 0, len(order.Items)),
		Total:        order.Total,
		Currency:     order.Currency,
		CanCancel:    order.Status == orders.StatusPending && order.CanCancel,
		InvoiceID:    optionalOrderText(order.InvoiceID),
		TrackingCode: optionalOrderText(order.TrackingCode),
		CreatedAt:    order.CreatedAt.Format(time.RFC3339Nano),
		UpdatedAt:    order.UpdatedAt.Format(time.RFC3339Nano),
	}

	for _, item := range order.Items {
		result.Items = append(result.Items, orderItemResult{
			ProductID:   item.ProductID,
			ProductName: item.ProductName,
			Quantity:    int32(item.Quantity),
			UnitPrice:   item.UnitPrice,
		})
	}

	if order.ShippedAt != nil {
		value := order.ShippedAt.Format(time.RFC3339Nano)
		result.ShippedAt = &value
	}

	if order.CheckoutExpiresAt != nil {
		value := order.CheckoutExpiresAt.Format(time.RFC3339Nano)
		result.CheckoutExpiresAt = &value

		if order.Status == orders.StatusAwaitingPayment && order.CheckoutExpiresAt.After(now) {
			link, err := url.Parse(order.CheckoutURL)
			if err == nil &&
				(link.Scheme == "http" || link.Scheme == "https") &&
				link.Hostname() != "" &&
				link.User == nil {
				result.CheckoutURL = &order.CheckoutURL
			}
		}
	}

	return result
}

func optionalOrderText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
