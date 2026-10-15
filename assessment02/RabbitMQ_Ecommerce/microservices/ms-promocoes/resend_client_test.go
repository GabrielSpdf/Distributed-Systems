package mspromocoes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestResendClientSend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodPost || request.URL.Path != "/emails" {
			t.Errorf("requisição incorreta: %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer chave-ficticia" {
			t.Error("autorização incorreta")
		}
		if request.Header.Get("Content-Type") != "application/json" {
			t.Error("tipo de conteúdo incorreto")
		}
		if request.Header.Get("Idempotency-Key") != "teste-email-001" {
			t.Error("chave de idempotência incorreta")
		}

		var body struct {
			From    string   `json:"from"`
			To      []string `json:"to"`
			Subject string   `json:"subject"`
			Text    string   `json:"text"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("JSON inválido: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.From != "Event Market <onboarding@resend.dev>" ||
			len(body.To) != 1 || body.To[0] != "cliente@example.com" ||
			body.Subject != "Promoção de teste" || body.Text != "Conteúdo de teste" {
			t.Errorf("conteúdo incorreto: %+v", body)
		}

		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"id":"email-local-001"}`)
	}))
	defer server.Close()

	client, err := NewResendClient(
		"chave-ficticia",
		"Event Market <onboarding@resend.dev>",
		"cliente@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	client.endpoint = server.URL + "/emails"

	id, err := client.Send(context.Background(), EmailMessage{
		To:      "cliente@example.com",
		Subject: "Promoção de teste",
		Text:    "Conteúdo de teste",
	}, "teste-email-001")
	if err != nil {
		t.Fatal(err)
	}
	if id != "email-local-001" {
		t.Fatalf("identificador incorreto: %s", id)
	}
}

func TestResendClientBlocksOtherRecipient(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(
		writer http.ResponseWriter,
		_ *http.Request,
	) {
		calls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := NewResendClient(
		"chave-ficticia",
		"onboarding@resend.dev",
		"cliente@example.com",
	)
	if err != nil {
		t.Fatal(err)
	}
	client.endpoint = server.URL + "/emails"

	_, err = client.Send(context.Background(), EmailMessage{
		To:      "outra-pessoa@example.com",
		Subject: "Teste",
		Text:    "Teste",
	}, "teste-email-002")
	if err == nil {
		t.Fatal("destinatário não permitido foi aceito")
	}
	if calls.Load() != 0 {
		t.Fatal("requisição enviada apesar do destinatário bloqueado")
	}
}

func TestResendClientHTTPError(t *testing.T) {
	tests := []struct {
		status    int
		temporary bool
	}{
		{403, false},
		{429, true},
		{500, true},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("HTTP_%d", test.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(
				writer http.ResponseWriter,
				_ *http.Request,
			) {
				writer.WriteHeader(test.status)
			}))
			defer server.Close()

			client, err := NewResendClient(
				"chave-ficticia",
				"onboarding@resend.dev",
				"cliente@example.com",
			)
			if err != nil {
				t.Fatal(err)
			}
			client.endpoint = server.URL + "/emails"

			_, err = client.Send(context.Background(), EmailMessage{
				To:      "cliente@example.com",
				Subject: "Teste",
				Text:    "Teste",
			}, "teste-email-003")

			var providerError *ResendHTTPError
			if !errors.As(err, &providerError) {
				t.Fatalf("esperava erro HTTP do Resend; erro=%v", err)
			}
			if providerError.StatusCode != test.status ||
				providerError.Temporary() != test.temporary {
				t.Fatalf("classificação incorreta: %+v", providerError)
			}
		})
	}
}
