package orders

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"RabbitMQ_Ecommerce/microservices/ms-gateway/auth"
)

func (handler *HTTPHandler) streamEvents(
	writer http.ResponseWriter,
	request *http.Request,
) {
	userID, authenticated := auth.UserIDFromContext(request.Context())
	if !authenticated {
		writeError(writer, http.StatusUnauthorized, "usuário não autenticado")
		return
	}

	controller := http.NewResponseController(writer)

	// Aplica um prazo próprio somente para esta resposta.
	if err := controller.SetWriteDeadline(
		time.Now().Add(5 * time.Second),
	); err != nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"streaming não disponível",
		)
		return
	}

	channel, unsubscribe := handler.repository.sseHub.subscribe(userID)
	defer unsubscribe()

	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("X-Accel-Buffering", "no")

	send := func(frame string) error {
		if err := controller.SetWriteDeadline(
			time.Now().Add(5 * time.Second),
		); err != nil {
			return err
		}

		if _, err := fmt.Fprint(writer, frame); err != nil {
			return err
		}

		if err := controller.Flush(); err != nil {
			return err
		}

		return controller.SetWriteDeadline(time.Time{})
	}

	if err := send("event: connection.ready\ndata: {}\n\n"); err != nil {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	// A reconexão passa novamente pela autenticação.
	lifetime := time.NewTimer(time.Minute)
	defer lifetime.Stop()

	for {
		select {
		case <-request.Context().Done():
			return

		case <-lifetime.C:
			return

		case <-heartbeat.C:
			if err := send("event: connection.heartbeat\ndata: {}\n\n"); err != nil {
				return
			}

		case event, open := <-channel:
			if !open {
				return
			}

			frame, err := formatOrderSSEEvent(event)
			if err != nil {
				return
			}

			if err := send(frame); err != nil {
				return
			}
		}
	}
}

func formatOrderSSEEvent(event orderSSEEvent) (string, error) {
	if event.Name == "" ||
		strings.ContainsAny(event.Name, "\r\n\x00") ||
		strings.ContainsAny(event.ID, "\r\n\x00") {
		return "", fmt.Errorf("nome ou identificador SSE inválido")
	}

	var data bytes.Buffer
	if err := json.Compact(&data, event.Data); err != nil {
		return "", fmt.Errorf("dados SSE inválidos: %w", err)
	}

	var frame strings.Builder

	if event.ID != "" {
		fmt.Fprintf(&frame, "id: %s\n", event.ID)
	}

	fmt.Fprintf(&frame, "event: %s\n", event.Name)
	fmt.Fprintf(&frame, "data: %s\n\n", data.String())

	return frame.String(), nil
}
