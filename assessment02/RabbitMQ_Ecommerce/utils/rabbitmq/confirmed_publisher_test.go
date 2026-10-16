package rabbitmq

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Usa exchange e fila próprias com nome aleatório, apagadas no fim, para não
// tocar nas filas reais do projeto no mesmo broker.
func openConfirmTest(t *testing.T) (*amqp.Connection, string) {
	t.Helper()

	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL não definida; teste de integração ignorado")
	}

	connection, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("erro ao conectar ao RabbitMQ de teste: %v", err)
	}
	t.Cleanup(func() { connection.Close() })

	channel, err := connection.Channel()
	if err != nil {
		t.Fatalf("erro ao abrir canal: %v", err)
	}
	t.Cleanup(func() { channel.Close() })

	exchange := "teste.confirmacao." + strconv.FormatInt(time.Now().UnixNano(), 36)

	if err := channel.ExchangeDeclare(exchange, "direct", false, true, false, false, nil); err != nil {
		t.Fatalf("erro ao declarar exchange de teste: %v", err)
	}
	t.Cleanup(func() { channel.ExchangeDelete(exchange, false, false) })

	return connection, exchange
}

func testEnvelope(eventType string) events.EventEnvelope {
	return events.EventEnvelope{
		EventID:   "evento-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		EventType: eventType,
		Producer:  "teste",
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"ok":true}`),
		Signature: "assinatura",
	}
}

func TestPublishConfirmedDeliversToBoundQueue(t *testing.T) {
	connection, exchange := openConfirmTest(t)

	channel, err := connection.Channel()
	if err != nil {
		t.Fatalf("erro ao abrir canal: %v", err)
	}
	defer channel.Close()

	queue, err := channel.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("erro ao declarar fila de teste: %v", err)
	}

	if err := channel.QueueBind(queue.Name, "evento.teste", exchange, false, nil); err != nil {
		t.Fatalf("erro ao ligar fila de teste: %v", err)
	}

	envelope := testEnvelope("evento.teste")

	if err := PublishConfirmed(context.Background(), connection, exchange, envelope); err != nil {
		t.Fatalf("publicação confirmada falhou: %v", err)
	}

	delivery, ok, err := channel.Get(queue.Name, true)
	if err != nil || !ok {
		t.Fatalf("mensagem não chegou à fila: ok=%v err=%v", ok, err)
	}

	var received events.EventEnvelope
	if err := json.Unmarshal(delivery.Body, &received); err != nil {
		t.Fatalf("corpo inválido: %v", err)
	}

	if received.EventID != envelope.EventID || delivery.MessageId != envelope.EventID ||
		delivery.DeliveryMode != amqp.Persistent {
		t.Fatalf("mensagem inesperada: %+v", delivery)
	}
}

func TestPublishConfirmedFailsWithoutRoute(t *testing.T) {
	connection, exchange := openConfirmTest(t)

	err := PublishConfirmed(context.Background(), connection, exchange, testEnvelope("evento.sem.fila"))
	if err == nil {
		t.Fatal("publicação sem fila de destino deveria falhar")
	}
}

func TestPublishConfirmedFailsWithClosedConnection(t *testing.T) {
	_, exchange := openConfirmTest(t)

	// Conexão separada: a da limpeza precisa continuar aberta.
	closed, err := amqp.Dial(os.Getenv("TEST_RABBITMQ_URL"))
	if err != nil {
		t.Fatalf("erro ao conectar ao RabbitMQ de teste: %v", err)
	}
	closed.Close()

	if err := PublishConfirmed(context.Background(), closed, exchange, testEnvelope("evento.teste")); err == nil {
		t.Fatal("publicação com conexão fechada deveria falhar")
	}
}
