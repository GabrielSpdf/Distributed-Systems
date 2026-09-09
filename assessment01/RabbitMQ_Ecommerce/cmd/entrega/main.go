package main

import (
	"log"

	msentrega "RabbitMQ_Ecommerce/microservices/ms-entrega"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	runtime, err := msentrega.InitMSEntrega()
	if err != nil {
		return err
	}
	defer runtime.Connection.Close()
	defer runtime.Channel.Close()

	log.Printf(
		"Entrega aguardando eventos na fila %s",
		runtime.QueueName,
	)

	return rabbitmq.ConsumeEvents(
		runtime.Channel,
		runtime.QueueName,
		handleDeliveryEvent,
	)
}

// handleDeliveryEvent é o ponto de extensão para a regra de negócio da
// Entrega (nota fiscal, rastreio). TODO: implementar conforme o cronograma.
func handleDeliveryEvent(envelope events.EventEnvelope) error {
	log.Printf(
		"[TODO] Entrega recebeu evento %s (ainda não processado)",
		envelope.EventType,
	)
	return nil
}
