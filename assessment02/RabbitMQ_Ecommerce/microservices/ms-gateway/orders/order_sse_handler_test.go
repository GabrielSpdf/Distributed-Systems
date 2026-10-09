package orders

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFormatOrderSSEEvent(t *testing.T) {
	event := orderSSEEvent{
		Name: "order.status.changed",
		ID:   "evento-1",
		Data: json.RawMessage(`{
			"orderId": "PED-001",
			"status": "ESTOQUE_CONFIRMADO"
		}`),
	}

	frame, err := formatOrderSSEEvent(event)
	if err != nil {
		t.Fatal(err)
	}

	expected := "id: evento-1\n" +
		"event: order.status.changed\n" +
		"data: {\"orderId\":\"PED-001\",\"status\":\"ESTOQUE_CONFIRMADO\"}\n\n"

	if frame != expected {
		t.Fatalf("formato incorreto:\nrecebido: %q\nesperado: %q", frame, expected)
	}
}

func TestFormatOrderSSEEventWithoutID(t *testing.T) {
	frame, err := formatOrderSSEEvent(orderSSEEvent{
		Name: "connection.ready",
		Data: json.RawMessage(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}

	if frame != "event: connection.ready\ndata: {}\n\n" {
		t.Fatalf("formato incorreto: %q", frame)
	}
}

func TestFormatOrderSSEEventRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		event orderSSEEvent
	}{
		{
			name:  "nome vazio",
			event: orderSSEEvent{Data: json.RawMessage(`{}`)},
		},
		{
			name: "quebra de linha no nome",
			event: orderSSEEvent{
				Name: "evento\noutro",
				Data: json.RawMessage(`{}`),
			},
		},
		{
			name: "quebra de linha no identificador",
			event: orderSSEEvent{
				Name: "evento",
				ID:   "id\routro",
				Data: json.RawMessage(`{}`),
			},
		},
		{
			name: "JSON inválido",
			event: orderSSEEvent{
				Name: "evento",
				Data: json.RawMessage(`{"status":`),
			},
		},
		{
			name:  "dados ausentes",
			event: orderSSEEvent{Name: "evento"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := formatOrderSSEEvent(test.event); err == nil {
				t.Fatal("entrada inválida foi aceita")
			}
		})
	}
}

func TestOrderSSEEndpointRequiresAuthentication(t *testing.T) {
	handler := NewHTTPHandler(NewRepository(nil), nil, nil)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/orders/events",
		nil,
	)
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, recebido %d", response.Code)
	}
}
