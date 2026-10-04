package orders

import "time"

const (
	StatusPending          = "PENDENTE"
	StatusCancelled        = "CANCELADO"
	StatusStockUnavailable = "ESTOQUE_INDISPONIVEL"
	StatusStockConfirmed   = "ESTOQUE_CONFIRMADO"
	StatusAwaitingPayment  = "AGUARDANDO_PAGAMENTO"
	StatusPaymentRefused   = "PAGAMENTO_RECUSADO"
	StatusPaymentApproved  = "PAGAMENTO_APROVADO"
	StatusShipped          = "ENVIADO"
	StatusProcessingFailed = "FALHA_NO_PROCESSAMENTO"
)

type CreateOrderRequest struct {
	Items []CreateOrderItem `json:"items"`
}

type CreateOrderItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type OrderItem struct {
	ProductID   string  `json:"product_id"`
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

type Link struct {
	Href   string `json:"href"`
	Method string `json:"method"`
}

type Order struct {
	DatabaseID  string          `json:"-"`
	UserID      string          `json:"-"`
	ID          string          `json:"id"`
	Status      string          `json:"status"`
	Items       []OrderItem     `json:"items"`
	Total       float64         `json:"total"`
	Currency    string          `json:"currency"`
	CheckoutURL string          `json:"checkout_url,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Links       map[string]Link `json:"_links"`
}
