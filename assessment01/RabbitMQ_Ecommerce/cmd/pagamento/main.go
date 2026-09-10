package main

import (
	"log"

	"RabbitMQ_Ecommerce/microservices/ms-pagamento"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	msPagamento, err := mspagamento.InitMSPagamento()
	if err != nil {
		return err
	}
	defer msPagamento.Connection.Close()
	defer msPagamento.Channel.Close()

	log.Printf(
		"[✓] Pagamento aguardando eventos na fila %s",
		msPagamento.QueueName,
	)

	return rabbitmq.ConsumeEvents(
		msPagamento.Channel,
		msPagamento.QueueName,
		handlePaymentEvent,
	)
}

// handlePaymentEvent é o ponto de extensão para a regra de negócio do
// Pagamento (aprovação/recusa). TODO: implementar conforme o cronograma.
func handlePaymentEvent(envelope events.EventEnvelope) error {
	log.Printf(
		"[TODO] Pagamento recebeu evento %s (ainda não processado)",
		envelope.EventType,
	)
	return nil
}
