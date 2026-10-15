package mspromocoes

import (
	"context"
	"testing"
)

func TestEmailWorkerRequiresSender(t *testing.T) {
	repository := NewEmailRepository(nil)

	err := repository.RunEmailWorker(
		context.Background(), "", "", nil,
	)
	if err == nil {
		t.Fatal("esperava erro ao iniciar sem remetente")
	}

	handled, err := repository.ProcessNextEmailJob(
		context.Background(), "", "", nil,
	)
	if err == nil || handled {
		t.Fatalf("handled=%v, erro=%v; esperado false e erro", handled, err)
	}
}

func TestEmailWorkerAlreadyCancelled(t *testing.T) {
	repository := NewEmailRepository(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	err := repository.RunEmailWorker(
		ctx,
		"Event Market <onboarding@resend.dev>",
		"cliente@example.com",
		func(context.Context, EmailMessage, string) (string, error) {
			calls++
			return "nao-deveria-enviar", nil
		},
	)
	if err != nil {
		t.Fatalf("cancelamento deveria encerrar normalmente: %v", err)
	}
	if calls != 0 {
		t.Fatalf("houve %d chamadas após cancelamento", calls)
	}
}
