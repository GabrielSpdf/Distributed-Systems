package events

type Product struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
}

type OrderItem struct {
	ProductID  string  `json:"product_id"`
	Name      string  `json:"name"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type Order struct {
	ID         string      `json:"id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total 	   float64     `json:"total"`
	Status     OrderStatus `json:"status"`
}

type OrderStatus string

const (
	StatusCreated          OrderStatus = "CRIADO"
	StatusStockReserved    OrderStatus = "ESTOQUE_RESERVADO"
	StatusStockUnavailable OrderStatus = "ESTOQUE_INDISPONIVEL"
	StatusPaymentApproved  OrderStatus = "PAGAMENTO_APROVADO"
	StatusPaymentRefused   OrderStatus = "PAGAMENTO_RECUSADO"
	StatusCancelled        OrderStatus = "CANCELADO"
	StatusShipped          OrderStatus = "ENVIADO"
)

type OrderCreatedPayload struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total      float64     `json:"total"`
}

type OrderReferencePayload struct {
	OrderID    string      `json:"order_id"`
}

type StockUnavailablePayload struct {
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Reason    string `json:"reason"`
}

type PaymentResultPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason,omitempty"`
}

type OrderShippedPayload struct {
	OrderID      string `json:"order_id"`
	InvoiceId    string `json:"invoice_id"`
	TrackingCode string `json:"tracking_code"`
}

