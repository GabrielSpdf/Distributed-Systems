package mspromocoes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

type EmailMessage struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

type ResendClient struct {
	apiKey        string
	from          string
	testRecipient string
	endpoint      string
	httpClient    *http.Client
}

func NewResendClient(
	apiKey string,
	from string,
	testRecipient string,
) (*ResendClient, error) {
	apiKey = strings.TrimSpace(apiKey)
	from = strings.TrimSpace(from)
	testRecipient = strings.TrimSpace(testRecipient)

	if apiKey == "" || strings.ContainsAny(apiKey, "\r\n") {
		return nil, fmt.Errorf("chave do Resend ausente ou inválida")
	}

	if _, err := mail.ParseAddress(from); err != nil {
		return nil, fmt.Errorf("remetente inválido")
	}

	address, err := mail.ParseAddress(testRecipient)
	if err != nil || address.Address != testRecipient {
		return nil, fmt.Errorf("destinatário de teste inválido")
	}

	return &ResendClient{
		apiKey:        apiKey,
		from:          from,
		testRecipient: testRecipient,
		endpoint:      "https://api.resend.com/emails",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(
				_ *http.Request,
				_ []*http.Request,
			) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

type ResendHTTPError struct {
	StatusCode int
}

func (err *ResendHTTPError) Error() string {
	return fmt.Sprintf("Resend recusou o envio: HTTP %d", err.StatusCode)
}

func (err *ResendHTTPError) Temporary() bool {
	return err.StatusCode == 408 ||
		err.StatusCode == 429 ||
		(err.StatusCode >= 500 && err.StatusCode <= 599)
}

// Send retorna o identificador do e-mail aceito pelo Resend.
func (client *ResendClient) Send(
	ctx context.Context,
	message EmailMessage,
	idempotencyKey string,
) (string, error) {
	if client == nil || client.httpClient == nil {
		return "", fmt.Errorf("cliente Resend não configurado")
	}

	recipient := strings.TrimSpace(message.To)
	address, err := mail.ParseAddress(recipient)
	if err != nil || address.Address != recipient {
		return "", fmt.Errorf("destinatário inválido")
	}

	if !strings.EqualFold(recipient, client.testRecipient) {
		return "", fmt.Errorf("destinatário não permitido no modo de teste")
	}

	if strings.TrimSpace(message.Subject) == "" ||
		strings.ContainsAny(message.Subject, "\r\n") {
		return "", fmt.Errorf("assunto ausente ou inválido")
	}
	if strings.TrimSpace(message.Text) == "" &&
		strings.TrimSpace(message.HTML) == "" {
		return "", fmt.Errorf("conteúdo do e-mail ausente")
	}

	if strings.TrimSpace(idempotencyKey) == "" ||
		len(idempotencyKey) > 256 ||
		strings.ContainsAny(idempotencyKey, "\r\n") {
		return "", fmt.Errorf("chave de idempotência inválida")
	}

	payload := struct {
		From    string   `json:"from"`
		To      []string `json:"to"`
		Subject string   `json:"subject"`
		Text    string   `json:"text,omitempty"`
		HTML    string   `json:"html,omitempty"`
	}{
		From:    client.from,
		To:      []string{recipient},
		Subject: message.Subject,
		Text:    message.Text,
		HTML:    message.HTML,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("erro ao montar e-mail")
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		client.endpoint,
		bytes.NewReader(data),
	)
	if err != nil {
		return "", fmt.Errorf("erro ao preparar requisição de envio")
	}
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", idempotencyKey)

	response, err := client.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("envio interrompido: %w", ctx.Err())
		}
		// Não inclui detalhes da requisição nem a chave no erro.
		return "", fmt.Errorf("falha de comunicação com o Resend")
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", &ResendHTTPError{StatusCode: response.StatusCode}
	}

	const maxResponseBytes = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta do Resend")
	}
	if len(body) > maxResponseBytes {
		return "", fmt.Errorf("resposta do Resend excedeu o limite")
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("resposta inválida do Resend")
	}
	if strings.TrimSpace(result.ID) == "" {
		return "", fmt.Errorf("Resend respondeu sem identificador do e-mail")
	}

	return result.ID, nil
}
