package msestoque

import "time"

// Product is one catalog entry returned by the internal endpoint.
type Product struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
}

// ProductsResponse is the body of GET /internal/products.
type ProductsResponse struct {
	Products []Product `json:"products"`
	Total    int       `json:"total"`
}

// ProductFilter holds the optional catalog filters. A nil Available means
// "do not filter by availability".
type ProductFilter struct {
	Category  string
	Available *bool
}

// ReservationItem is a product and the quantity an order wants.
type ReservationItem struct {
	ProductID string
	Quantity  int
}

// Shortfall describes one product that cannot be fully supplied.
type Shortfall struct {
	ProductID         string
	RequestedQuantity int
	AvailableQuantity int
}

const (
	ReservationReserved    = "RESERVED"
	ReservationUnavailable = "UNAVAILABLE"
	ReservationReleased    = "RELEASED"
)

// ReservationResult is the stored outcome for one order.
type ReservationResult struct {
	Status     string
	Shortfalls []Shortfall
	ReservedAt time.Time
}
