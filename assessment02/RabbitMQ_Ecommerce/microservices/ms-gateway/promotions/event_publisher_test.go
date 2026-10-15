package promotions

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

func TestPublishConfirmedInterestIntegration(t *testing.T) {
	rabbitURL := os.Getenv("TEST_RABBITMQ_URL")
	if rabbitURL == "" {
		t.Skip("defina TEST_RABBITMQ_URL para executar a integração")
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

	randomID := make([]byte, 16)
	if _, err := rand.Read(randomID); err != nil {
		t.Fatal(err)
	}
	testID := hex.EncodeToString(randomID)
	exchangeName := "teste.promocoes." + testID

	if err := channel.ExchangeDeclare(
		exchangeName, "direct", false, false, false, false, nil,
	); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := channel.ExchangeDelete(exchangeName, false, false); err != nil {
			t.Errorf("limpeza da exchange de teste: %v", err)
		}
	}()

	queue, err := channel.QueueDeclare(
		"", false, true, true, false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := channel.QueueBind(
		queue.Name, events.PromotionInterest, exchangeName, false, nil,
	); err != nil {
		t.Fatal(err)
	}

	// Testa transporte. A assinatura real já possui testes próprios.
	envelope := events.EventEnvelope{
		EventID:   testID,
		EventType: events.PromotionInterest,
		Producer:  events.ProducerPrincipal,
		Timestamp: time.Now().UTC(),
		Payload:   json.RawMessage(`{"test":true}`),
		Signature: "assinatura-apenas-para-teste-de-transporte",
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}

	if err := publishConfirmedInterestToExchange(
		ctx, connection, exchangeName, data,
	); err != nil {
		t.Fatalf("publicação com destino: %v", err)
	}

	delivery, received, err := channel.Get(queue.Name, true)
	if err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Fatal("mensagem confirmada não chegou à fila")
	}
	if string(delivery.Body) != string(data) ||
		delivery.MessageId != envelope.EventID {
		t.Fatal("conteúdo recebido difere do envelope enviado")
	}

	// Remove somente o binding criado por este teste.
	if err := channel.QueueUnbind(
		queue.Name, events.PromotionInterest, exchangeName, nil,
	); err != nil {
		t.Fatal(err)
	}

	err = publishConfirmedInterestToExchange(
		ctx, connection, exchangeName, data,
	)
	if err == nil || !strings.Contains(err.Error(), "mensagem devolvida") {
		t.Fatalf("esperava devolução por ausência de destino; erro=%v", err)
	}

	if err := PublishConfirmedInterest(ctx, nil, data); err == nil {
		t.Fatal("conexão ausente foi aceita")
	}
}
