package events

// Define as constantes para os dois tipos de Exchange obrigatórias
const (
	ExchangeEcommerce = "eCommerce"
	ExchangePromocoes = "Promocoes"
)

// Define as constantes para os eventos obrigatórios que serão publicados/consumidos pela Exchange eCommerce (Tipo Direct)
const (
	PedidoCriado        = "pedido.criado"
	PedidoEstoqueOk     = "pedido.estoque_ok"
	PedidoExcluido      = "pedido.excluido"
	PedidoEnviado       = "pedido.enviado"
	EstoqueIndisponivel = "estoque.indisponivel"
	PagamentoAprovado   = "pagamento.aprovado"
	PagamentoRecusado   = "pagamento.recusado"
)

// Define as constantes para os eventos obrigatórios que serão publicados/consumidos pela Exchange Promoções (Tipo Topic)
const (
	PromocaoCategoriaA = "promocao.categoria.A"
	PromocaoCategoriaB = "promocao.categoria.B"
	PromocaoCategoriaC = "promocao.categoria.C"
)