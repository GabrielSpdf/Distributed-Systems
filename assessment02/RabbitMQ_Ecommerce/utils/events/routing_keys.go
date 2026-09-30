package events

// Define as constantes para os dois tipos de Exchange obrigatórias
const (
	ExchangeEcommerce  = "eCommerce"
	ExchangePromotions = "Promocoes"
)

// Define as constantes para os eventos obrigatórios que serão publicados/consumidos pela Exchange eCommerce (Tipo Direct)
const (
	OrderCreated        = "pedido.criado"
	OrderStockConfirmed = "pedido.estoque_ok"
	OrderDeleted        = "pedido.excluido"
	OrderShipped        = "pedido.enviado"
	StockUnavailable    = "estoque.indisponivel"
	PaymentApproved     = "pagamento.aprovado"
	PaymentRefused      = "pagamento.recusado"
)

// Define as constantes para os eventos obrigatórios que serão publicados/consumidos pela Exchange Promoções (Tipo Topic)
const (
	PromotionCategoryA = "promocao.categoria.limpeza"
	PromotionCategoryB = "promocao.categoria.alimentos"
	PromotionCategoryC = "promocao.categoria.eletronicos"
	AllPromotions      = "promocao.categoria.*"
)

const (
	ProducerPrincipal  = "ms-principal"
	ProducerPromotions = "ms-promocoes"
	ProducerStock      = "ms-estoque"
	ProducerPayment    = "ms-pagamento"
	ProducerDelivery   = "ms-entrega"
)
