package events

var expectedProducerByEventType = map[string]string{
	PedidoCriado:        ProducerPrincipal,
	PedidoExcluido:      ProducerPrincipal,
	PedidoEstoqueOk:     ProducerEstoque,
	EstoqueIndisponivel: ProducerEstoque,
	PagamentoAprovado:   ProducerPagamento,
	PagamentoRecusado:   ProducerPagamento,
	PedidoEnviado:       ProducerEntrega,
}

// ExpectedProducerFor returns the producer authorized to publish an event type.
func ExpectedProducerFor(eventType string) (string, bool) {
	producer, exists := expectedProducerByEventType[eventType]
	return producer, exists
}