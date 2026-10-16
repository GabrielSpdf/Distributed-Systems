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
	ProductID         string `json:"product_id"`
	RequestedQuantity int    `json:"requested_quantity"`
	AvailableQuantity int    `json:"available_quantity"`
}

const (
	ReservationReserved    = "RESERVED"
	ReservationUnavailable = "UNAVAILABLE"
	ReservationReleased    = "RELEASED"
)

// ReservationResult is the stored outcome for one order. Envelope is the
// signed result event already stored for it (empty until the first one is
// saved) and Published tells whether the broker confirmed it.
type ReservationResult struct {
	Status     string
	Shortfalls []Shortfall
	ReservedAt time.Time
	Envelope   string
	Published  bool
}

// PendingResult is a stored result event not yet confirmed by the broker.
type PendingResult struct {
	OrderID  string
	Envelope string
}
