package orders

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOrderSSEStreamIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("defina TEST_DATABASE_URL para executar o teste")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var databaseName string
	if err := pool.QueryRow(ctx, "SELECT current_database()").
		Scan(&databaseName); err != nil {
		t.Fatal(err)
	}
	if databaseName != "ecommerce_events_test" {
		t.Fatal("execute somente no banco ecommerce_events_test")
	}

	var userID string
	err = pool.QueryRow(ctx, `
		INSERT INTO gateway.users (name, email, password_hash)
		VALUES (
			'Teste SSE',
			gen_random_uuid()::text || '@example.test',
			'conta-exclusiva-de-teste'
		)
		RETURNING id
	`).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		if _, err := pool.Exec(cleanupCtx,
			"DELETE FROM gateway.users WHERE id = $1", userID,
		); err != nil {
			t.Errorf("erro ao limpar usuário de teste: %v", err)
		}
	}()

	tokens, err := auth.NewTokenManager(strings.Repeat("s", 48))
	if err != nil {
		t.Fatal(err)
	}

	token, err := tokens.Generate(userID)
	if err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(pool)
	orderHandler := NewHTTPHandler(repository, nil, nil)
	authHandler := auth.NewHTTPHandler(
		auth.NewUserRepository(pool),
		tokens,
	)

	server := httptest.NewServer(
		authHandler.RequireAuth(orderHandler.Routes()),
	)
	defer server.Close()

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		server.URL+"/api/orders/events",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	request.AddCookie(&http.Cookie{
		Name:  "session",
		Value: token,
	})

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("esperado 200, recebido %d", response.StatusCode)
	}

	for name, expected := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"X-Accel-Buffering": "no",
	} {
		if received := response.Header.Get(name); received != expected {
			t.Fatalf("cabeçalho %s: recebido %q", name, received)
		}
	}

	reader := bufio.NewReader(response.Body)

	readFrame := func() string {
		t.Helper()

		var frame strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				t.Fatalf("erro ao receber SSE: %v", err)
			}
			frame.WriteString(line)
			if line == "\n" {
				return frame.String()
			}
		}
	}

	if frame := readFrame(); frame != "event: connection.ready\ndata: {}\n\n" {
		t.Fatalf("confirmação incorreta: %q", frame)
	}

	// Este evento não deve chegar à conexão do usuário autenticado.
	repository.sseHub.publish("outro-cliente", orderSSEEvent{
		Name: "order.status.changed",
		ID:   "evento-outro",
		Data: json.RawMessage(`{"orderId":"PED-OUTRO"}`),
	})

	event := orderSSEEvent{
		Name: "order.status.changed",
		ID:   "evento-sse",
		Data: json.RawMessage(`{"orderId":"PED-TESTE","status":"CANCELADO"}`),
	}
	repository.sseHub.publish(userID, event)

	expected, err := formatOrderSSEEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if frame := readFrame(); frame != expected {
		t.Fatalf("evento incorreto: %q", frame)
	}

	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}

	// O servidor deve remover a assinatura após a desconexão.
	deadline := time.Now().Add(3 * time.Second)
	for {
		repository.sseHub.mutex.Lock()
		remaining := len(repository.sseHub.subscribers[userID])
		repository.sseHub.mutex.Unlock()

		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("assinatura permaneceu após a desconexão")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
