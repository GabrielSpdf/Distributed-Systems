package msprincipal

import (
	"sync"

	"RabbitMQ_Ecommerce/utils/events"
)

// OrderRepository coordinates access to the orders file.
type OrderRepository struct {
	filePath string
	mutex    sync.Mutex
}

// NewOrderRepository creates a repository for the specified orders file.
func NewOrderRepository(
	filePath string,
) *OrderRepository {
	return &OrderRepository{
		filePath: filePath,
	}
}

// Load loads the stored orders.
func (repository *OrderRepository) Load() (
	OrdersData,
	error,
) {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()

	return LoadOrders(repository.filePath)
}

// Add adds an order to the repository.
func (repository *OrderRepository) Add(
	order events.Order,
) error {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()

	return AddOrder(
		repository.filePath,
		order,
	)
}

// UpdateStatus updates an existing order status.
func (repository *OrderRepository) UpdateStatus(
	orderID string,
	status events.OrderStatus,
) error {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()

	return UpdateOrderStatus(
		repository.filePath,
		orderID,
		status,
	)
}

// Find finds an order by its identifier.
func (repository *OrderRepository) Find(
	orderID string,
) (events.Order, error) {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()

	return FindOrder(
		repository.filePath,
		orderID,
	)
}

// MarkAsDeleted logically deletes an order.
func (repository *OrderRepository) MarkAsDeleted(
	orderID string,
) error {
	repository.mutex.Lock()
	defer repository.mutex.Unlock()

	return MarkOrderAsDeleted(
		repository.filePath,
		orderID,
	)
}
