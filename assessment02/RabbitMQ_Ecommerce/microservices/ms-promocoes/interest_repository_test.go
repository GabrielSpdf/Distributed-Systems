package mspromocoes

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestApplyInterestEventIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("defina TEST_DATABASE_URL para executar a integração")
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if config.ConnConfig.Database != "ecommerce_events_test" {
		t.Fatal("use somente o banco isolado ecommerce_events_test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var userID string
	if err := pool.QueryRow(
		ctx, "SELECT gen_random_uuid()::text",
	).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, query := range []string{
			"DELETE FROM promocoes.email_logs WHERE user_id = $1",
			"DELETE FROM promocoes.email_jobs WHERE user_id = $1",
			"DELETE FROM promocoes.interest_inbox WHERE user_id = $1",
			"DELETE FROM promocoes.promotion_interests WHERE user_id = $1",
		} {
			if _, err := pool.Exec(cleanupCtx, query, userID); err != nil {
				t.Errorf("limpeza: %v", err)
			}
		}
	}()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	registry := cryptography.PublicKeyRegistry{
		events.ProducerPrincipal: &privateKey.PublicKey,
	}
	repository := NewInterestRepository(pool)

	build := func(action string, categories []string) events.EventEnvelope {
		t.Helper()

		envelope, err := misc.BuildSignedEnvelope(
			events.PromotionInterestPayload{
				UserID:     userID,
				Email:      "cliente@example.com",
				Action:     action,
				Categories: categories,
				OccurredAt: time.Now().UTC(),
			},
			events.PromotionInterest,
			events.ProducerPrincipal,
			privateKey,
		)
		if err != nil {
			t.Fatal(err)
		}
		return envelope
	}

	assertCounts := func(wantActive, wantEvents int) {
		t.Helper()

		var active, processed int
		err := pool.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM promocoes.promotion_interests
				 WHERE user_id = $1 AND active = TRUE),
				(SELECT COUNT(*) FROM promocoes.interest_inbox
				 WHERE user_id = $1)
		`, userID).Scan(&active, &processed)
		if err != nil {
			t.Fatal(err)
		}
		if active != wantActive || processed != wantEvents {
			t.Fatalf(
				"ativos=%d, eventos=%d; esperado=%d/%d",
				active, processed, wantActive, wantEvents,
			)
		}
	}

	subscription := build(
		events.PromotionSubscribe,
		[]string{"alimentos", "limpeza"},
	)
	applied, err := repository.ApplyInterestEvent(
		ctx, subscription, events.PromotionInterest, registry,
	)
	if err != nil || !applied {
		t.Fatalf("inscrição: aplicado=%v, erro=%v", applied, err)
	}
	assertCounts(2, 1)

	cancellation := build(
		events.PromotionUnsubscribe,
		[]string{"limpeza"},
	)
	applied, err = repository.ApplyInterestEvent(
		ctx, cancellation, events.PromotionInterest, registry,
	)
	if err != nil || !applied {
		t.Fatalf("cancelamento: aplicado=%v, erro=%v", applied, err)
	}
	assertCounts(1, 2)

	// Repetir a inscrição antiga não pode desfazer o cancelamento.
	applied, err = repository.ApplyInterestEvent(
		ctx, subscription, events.PromotionInterest, registry,
	)
	if err != nil || applied {
		t.Fatalf("evento antigo repetido: aplicado=%v, erro=%v", applied, err)
	}
	assertCounts(1, 2)

	tampered := build(events.PromotionSubscribe, []string{"eletronicos"})
	tampered.Payload = json.RawMessage(`{"categories":["limpeza"]}`)
	applied, err = repository.ApplyInterestEvent(
		ctx, tampered, events.PromotionInterest, registry,
	)
	if applied || !errors.Is(err, ErrInvalidInterestEvent) {
		t.Fatalf("evento adulterado: aplicado=%v, erro=%v", applied, err)
	}
	assertCounts(1, 2)

	// Uma nova inscrição, com outro event_id, pode reativar a categoria.
	resubscription := build(
		events.PromotionSubscribe,
		[]string{"limpeza"},
	)
	applied, err = repository.ApplyInterestEvent(
		ctx, resubscription, events.PromotionInterest, registry,
	)
	if err != nil || !applied {
		t.Fatalf("nova inscrição: aplicado=%v, erro=%v", applied, err)
	}
	assertCounts(2, 3)

	t.Run("cancelamento_dos_envios_pendentes", func(t *testing.T) {
		var jobID string
		err := pool.QueryRow(ctx, `
			INSERT INTO promocoes.email_jobs (
				promotion_event_id, user_id, category,
				from_address, recipient, subject,
				text_body, idempotency_key
			)
			VALUES (
				gen_random_uuid(), $1, 'limpeza',
				'onboarding@resend.dev', 'cliente@example.com',
				'Oferta de teste', 'Conteúdo de teste',
				'test/' || gen_random_uuid()::text
			)
			RETURNING id::text
		`, userID).Scan(&jobID)
		if err != nil {
			t.Fatal(err)
		}

		cancelEvent := build(
			events.PromotionUnsubscribe,
			[]string{"limpeza"},
		)
		applied, err := repository.ApplyInterestEvent(
			ctx, cancelEvent, events.PromotionInterest, registry,
		)
		if err != nil || !applied {
			t.Fatalf("cancelamento: aplicado=%v, erro=%v", applied, err)
		}

		var jobStatus string
		var attempts int
		err = pool.QueryRow(ctx, `
			SELECT status, attempt_count
			FROM promocoes.email_jobs
			WHERE id = $1
		`, jobID).Scan(&jobStatus, &attempts)
		if err != nil {
			t.Fatal(err)
		}
		if jobStatus != "CANCELLED" || attempts != 0 {
			t.Fatalf(
				"envio após cancelamento: status=%s, tentativas=%d",
				jobStatus, attempts,
			)
		}

		// Reativar o interesse não deve ressuscitar uma oferta cancelada.
		newSubscription := build(
			events.PromotionSubscribe,
			[]string{"limpeza"},
		)
		applied, err = repository.ApplyInterestEvent(
			ctx, newSubscription, events.PromotionInterest, registry,
		)
		if err != nil || !applied {
			t.Fatalf("reativação: aplicado=%v, erro=%v", applied, err)
		}

		err = pool.QueryRow(ctx, `
			SELECT status
			FROM promocoes.email_jobs
			WHERE id = $1
		`, jobID).Scan(&jobStatus)
		if err != nil {
			t.Fatal(err)
		}
		if jobStatus != "CANCELLED" {
			t.Fatal("reativação recuperou um envio já cancelado")
		}
	})
}
