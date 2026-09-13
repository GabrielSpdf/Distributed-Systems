package events

// Representa a estrutura de um produto disponível no catálogo do e-commerce
type Product struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Category string  `json:"category"`
	Price    float64 `json:"price"`
}

// Representa um produto e sua respectiva quantidade dentro de um pedido
type OrderItem struct {
	Product  Product `json:"product"`
	Quantity int     `json:"quantity"`
	Price    float64 `json:"price"`
}

// Representa um pedido mantido pelo microsserviço Principal
type Order struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total      float64     `json:"total"`
	Status     OrderStatus `json:"status"`
}

// Representa um possível estado de processamento de um pedido
type OrderStatus string

const (
	StatusCreated          OrderStatus = "CRIADO"
	StatusStockReserved    OrderStatus = "ESTOQUE_RESERVADO"
	StatusStockUnavailable OrderStatus = "ESTOQUE_INDISPONIVEL"
	StatusPaymentApproved  OrderStatus = "PAGAMENTO_APROVADO"
	StatusPaymentRefused   OrderStatus = "PAGAMENTO_RECUSADO"
	StatusCancelled        OrderStatus = "CANCELADO"
	StatusShipped          OrderStatus = "ENVIADO"
	StatusPending          OrderStatus = "PENDENTE"
	StatusDeleted          OrderStatus = "EXCLUÍDO"
)

// Representa o conteúdo do evento pedido.criado
type OrderCreatedPayload struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total      float64     `json:"total"`
}

// Representa o conteúdo dos eventos que precisam apenas identificar o pedido relacionado.
type OrderReferencePayload struct {
	OrderID string `json:"order_id"`
}

// Rrepresenta o conteúdo do evento estoque.indisponivel
type StockUnavailablePayload struct {
	OrderID   string `json:"order_id"`
	ProductID string `json:"product_id"`
	Reason    string `json:"reason"`
}

// Representa o resultado do processamento de um pagamento
type PaymentResultPayload struct {
	OrderID string `json:"order_id"`
	Reason  string `json:"reason,omitempty"`
}

// Representa o conteúdo do evento pedido.enviado
type OrderShippedPayload struct {
	OrderID      string `json:"order_id"`
	InvoiceID    string `json:"invoice_id"`
	TrackingCode string `json:"tracking_code"`
}

type InventoryData struct {
	Products []Product      `json:"products"`
	Stock    map[string]int `json:"stock"`
}
