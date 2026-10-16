package rabbitmq

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"

	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	testKeyOnce sync.Once
	testKey     *rsa.PrivateKey
)

func signingKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	testKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("erro ao gerar chave de teste: %v", err)
		}
		testKey = key
	})

	return testKey
}

// consumerFixture cria exchange e fila temporárias com nomes únicos e as
// remove no fim; nenhuma fila real do projeto é usada.
type consumerFixture struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	queue      string
	keys       cryptography.PublicKeyRegistry
}

func newConsumerFixture(t *testing.T) *consumerFixture {
	t.Helper()

	url := os.Getenv("TEST_RABBITMQ_URL")
	if url == "" {
		t.Skip("TEST_RABBITMQ_URL não definida; teste de integração ignorado")
	}

	connection, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("erro ao conectar ao RabbitMQ de teste: %v", err)
	}

	admin, err := connection.Channel()
	if err != nil {
		t.Fatalf("erro ao abrir canal: %v", err)
	}

	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	fixture := &consumerFixture{
		connection: connection,
		exchange:   "teste.consumo." + suffix,
		queue:      "teste.consumo." + suffix,
		keys: cryptography.PublicKeyRegistry{
			events.ProducerPrincipal: &signingKey(t).PublicKey,
		},
	}

	// Limpeza registrada antes de criar qualquer recurso, para não deixar
	// exchange ou fila para trás se a preparação falhar no meio.
	t.Cleanup(func() {
		if fixture.channel != nil {
			fixture.channel.Close()
		}
		admin.QueueDelete(fixture.queue, false, false, false)
		admin.QueueDelete(ReviewQueueName(fixture.queue), false, false, false)
		admin.ExchangeDelete(fixture.exchange, false, false)
		admin.Close()
		connection.Close()
	})

	if err := admin.ExchangeDeclare(fixture.exchange, "direct", false, false, false, false, nil); err != nil {
		t.Fatalf("erro ao declarar exchange de teste: %v", err)
	}

	if _, err := admin.QueueDeclare(fixture.queue, true, false, false, false, nil); err != nil {
		t.Fatalf("erro ao declarar fila de teste: %v", err)
	}

	if err := admin.QueueBind(fixture.queue, events.OrderCreated, fixture.exchange, false, nil); err != nil {
		t.Fatalf("erro ao ligar fila de teste: %v", err)
	}

	consumerChannel, err := connection.Channel()
	if err != nil {
		t.Fatalf("erro ao abrir canal do consumidor: %v", err)
	}
	fixture.channel = consumerChannel

	return fixture
}

func (fixture *consumerFixture) publish(t *testing.T, envelope events.EventEnvelope) {
	t.Helper()

	if err := PublishConfirmed(context.Background(), fixture.connection, fixture.exchange, envelope); err != nil {
		t.Fatalf("erro ao publicar evento de teste: %v", err)
	}
}

func signedOrderCreated(t *testing.T) events.EventEnvelope {
	t.Helper()

	envelope, err := misc.BuildSignedEnvelope(
		map[string]string{"order_id": "PED-TESTE"},
		events.OrderCreated,
		events.ProducerPrincipal,
		signingKey(t),
	)
	if err != nil {
		t.Fatalf("erro ao assinar envelope: %v", err)
	}

	return envelope
}

var fastRetry = RetryOptions{Attempts: 3, BaseDelay: 10 * time.Millisecond, MaxDelay: 20 * time.Millisecond}

// consume roda o consumidor até o fim do teste.
func (fixture *consumerFixture) consume(
	t *testing.T,
	handler func(events.EventEnvelope) error,
) {
	t.Helper()

	go func() {
		_ = ConsumeSignedEventsReliably(
			fixture.connection,
			fixture.channel,
			fixture.queue,
			fixture.keys,
			handler,
			fastRetry,
		)
	}()
}

func (fixture *consumerFixture) count(t *testing.T, queue string) int {
	t.Helper()

	channel, err := fixture.connection.Channel()
	if err != nil {
		t.Fatalf("erro ao abrir canal: %v", err)
	}
	defer channel.Close()

	state, err := channel.QueueDeclarePassive(queue, true, false, false, false, nil)
	if err != nil {
		t.Fatalf("erro ao consultar fila %s: %v", queue, err)
	}

	return state.Messages
}

type callCounter struct {
	mu    sync.Mutex
	calls int
	ids   []string
}

func (counter *callCounter) record(envelope events.EventEnvelope) int {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	counter.calls++
	counter.ids = append(counter.ids, envelope.EventID)

	return counter.calls
}

func (counter *callCounter) total() int {
	counter.mu.Lock()
	defer counter.mu.Unlock()

	return counter.calls
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("condição não atingida em 5 s")
}

func TestReliableConsumerAcksSuccessfulMessage(t *testing.T) {
	fixture := newConsumerFixture(t)
	counter := &callCounter{}

	fixture.consume(t, func(envelope events.EventEnvelope) error {
		counter.record(envelope)
		return nil
	})
	fixture.publish(t, signedOrderCreated(t))

	waitFor(t, func() bool { return counter.total() == 1 })
	time.Sleep(100 * time.Millisecond)

	if fixture.count(t, fixture.queue) != 0 || fixture.count(t, ReviewQueueName(fixture.queue)) != 0 {
		t.Fatal("mensagem processada não deveria ficar em nenhuma fila")
	}
}

func TestReliableConsumerRetriesTemporaryFailure(t *testing.T) {
	fixture := newConsumerFixture(t)
	counter := &callCounter{}

	fixture.consume(t, func(envelope events.EventEnvelope) error {
		if counter.record(envelope) < 3 {
			return errors.New("banco fora do ar")
		}
		return nil
	})
	fixture.publish(t, signedOrderCreated(t))

	waitFor(t, func() bool { return counter.total() == 3 })
	time.Sleep(100 * time.Millisecond)

	if counter.total() != 3 || fixture.count(t, ReviewQueueName(fixture.queue)) != 0 {
		t.Fatalf("esperava sucesso na terceira tentativa sem revisão: chamadas=%d", counter.total())
	}
}

func TestReliableConsumerParksExhaustedMessageForReview(t *testing.T) {
	fixture := newConsumerFixture(t)
	counter := &callCounter{}
	envelope := signedOrderCreated(t)

	fixture.consume(t, func(received events.EventEnvelope) error {
		counter.record(received)
		return errors.New("mock fora do ar")
	})
	fixture.publish(t, envelope)

	review := ReviewQueueName(fixture.queue)
	waitFor(t, func() bool { return fixture.count(t, review) == 1 })

	if counter.total() != fastRetry.Attempts {
		t.Fatalf("esperava %d tentativas, houve %d", fastRetry.Attempts, counter.total())
	}

	channel, _ := fixture.connection.Channel()
	defer channel.Close()

	delivery, ok, err := channel.Get(review, true)
	if err != nil || !ok {
		t.Fatalf("mensagem não está na revisão: %v", err)
	}

	var parked events.EventEnvelope
	if err := json.Unmarshal(delivery.Body, &parked); err != nil || parked.EventID != envelope.EventID {
		t.Fatalf("mensagem original alterada: %v %+v", err, parked)
	}

	if delivery.Headers["x-revisao-motivo"] != "mock fora do ar" ||
		delivery.Headers["x-fila-origem"] != fixture.queue ||
		delivery.Headers["x-routing-key-origem"] != events.OrderCreated ||
		delivery.Headers["x-exchange-origem"] != fixture.exchange {
		t.Fatalf("cabeçalhos de revisão inesperados: %+v", delivery.Headers)
	}
}

func TestReliableConsumerDropsPermanentFailure(t *testing.T) {
	fixture := newConsumerFixture(t)
	counter := &callCounter{}

	fixture.consume(t, func(envelope events.EventEnvelope) error {
		counter.record(envelope)
		return Permanent(errors.New("payload inválido"))
	})
	fixture.publish(t, signedOrderCreated(t))

	waitFor(t, func() bool { return counter.total() == 1 })
	time.Sleep(150 * time.Millisecond)

	if counter.total() != 1 || fixture.count(t, ReviewQueueName(fixture.queue)) != 0 ||
		fixture.count(t, fixture.queue) != 0 {
		t.Fatalf("erro permanente deveria ser descartado sem novas tentativas: chamadas=%d", counter.total())
	}
}

func TestReliableConsumerRejectsTamperedSignature(t *testing.T) {
	fixture := newConsumerFixture(t)
	counter := &callCounter{}

	fixture.consume(t, func(envelope events.EventEnvelope) error {
		counter.record(envelope)
		return nil
	})

	envelope := signedOrderCreated(t)
	envelope.Payload = json.RawMessage(`{"order_id":"PED-ADULTERADO"}`)
	fixture.publish(t, envelope)

	waitFor(t, func() bool { return fixture.count(t, fixture.queue) == 0 })
	time.Sleep(150 * time.Millisecond)

	if counter.total() != 0 || fixture.count(t, ReviewQueueName(fixture.queue)) != 0 {
		t.Fatal("mensagem adulterada não pode chegar ao handler nem à revisão")
	}
}

func TestReprocessedMessageIsAccepted(t *testing.T) {
	fixture := newConsumerFixture(t)
	failing := true
	var mu sync.Mutex
	counter := &callCounter{}
	envelope := signedOrderCreated(t)

	fixture.consume(t, func(received events.EventEnvelope) error {
		counter.record(received)
		mu.Lock()
		defer mu.Unlock()
		if failing {
			return errors.New("banco fora do ar")
		}
		return nil
	})
	fixture.publish(t, envelope)

	review := ReviewQueueName(fixture.queue)
	waitFor(t, func() bool { return fixture.count(t, review) == 1 })

	mu.Lock()
	failing = false
	mu.Unlock()

	moved, err := ReprocessReview(context.Background(), fixture.connection, fixture.queue, 10)
	if err != nil || moved != 1 {
		t.Fatalf("esperava reprocessar 1 mensagem: %d %v", moved, err)
	}

	waitFor(t, func() bool { return counter.total() == fastRetry.Attempts+1 })
	time.Sleep(100 * time.Millisecond)

	counter.mu.Lock()
	lastID := counter.ids[len(counter.ids)-1]
	counter.mu.Unlock()

	if lastID != envelope.EventID || fixture.count(t, review) != 0 || fixture.count(t, fixture.queue) != 0 {
		t.Fatalf("reprocessamento inesperado: último=%s", lastID)
	}
}

func TestPermanentWrapping(t *testing.T) {
	base := errors.New("payload inválido")
	wrapped := Permanent(base)

	if !IsPermanent(wrapped) || !errors.Is(wrapped, base) || IsPermanent(base) || Permanent(nil) != nil {
		t.Fatal("Permanent/IsPermanent com comportamento inesperado")
	}

	if !IsPermanent(errors.Join(errors.New("contexto"), wrapped)) {
		t.Fatal("erro permanente embrulhado deveria continuar permanente")
	}
}
