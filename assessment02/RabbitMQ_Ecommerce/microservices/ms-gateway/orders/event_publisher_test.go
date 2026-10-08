package orders

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

func TestPublishConfirmedEventIntegration(t *testing.T) {
	rabbitURL := os.Getenv("TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		t.Skip("defina TEST_RABBITMQ_URL para executar este teste")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	connection, err := rabbitmq.ConnectURL(rabbitURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()

	channel, err := connection.Channel()
	if err != nil {
		t.Fatal(err)
	}
	defer channel.Close()

	if err := rabbitmq.DeclareExchange(
		channel, events.ExchangeEcommerce, rabbitmq.ExchangeTypeDirect,
	); err != nil {
		t.Fatal(err)
	}

	// Nome e rota exclusivos; não aciona consumidores de negócio.
	randomID := make([]byte, 16)
	if _, err := rand.Read(randomID); err != nil {
		t.Fatal(err)
	}
	testID := hex.EncodeToString(randomID)
	routingKey := "teste.outbox." + testID

	queue, err := channel.QueueDeclare(
		"", false, true, true, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := channel.QueueBind(
		queue.Name, routingKey, events.ExchangeEcommerce, false, nil,
	); err != nil {
		t.Fatal(err)
	}

	// Testa transporte, não assinatura: nenhuma chave real é necessária.
	envelope := events.EventEnvelope{
		EventID:   testID,
		EventType: routingKey,
		Producer:  events.ProducerPrincipal,
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"test":true}`),
		Signature: "teste-de-transporte",
	}

	if err := PublishConfirmedEvent(ctx, connection, envelope); err != nil {
		t.Fatalf("publicação com destino: %v", err)
	}

	delivery, received, err := channel.Get(queue.Name, true)
	if err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("mensagem confirmada não chegou à fila")
	}

	if delivery.MessageId != envelope.EventID ||
		string(delivery.Body) != mustMarshalEnvelope(t, envelope) {
		t.Fatal("mensagem recebida difere do envelope enviado")
	}

	// A rota seguinte não possui binding.
	envelope.EventType = routingKey + ".sem-destino"
	err = PublishConfirmedEvent(ctx, connection, envelope)
	if err == nil || !strings.Contains(err.Error(), "mensagem devolvida") {
		t.Fatalf("esperava devolução por ausência de destino; erro=%v", err)
	}

	if err := PublishConfirmedEvent(ctx, nil, envelope); err == nil {
		t.Fatal("publicação aceitou conexão ausente")
	}
}
