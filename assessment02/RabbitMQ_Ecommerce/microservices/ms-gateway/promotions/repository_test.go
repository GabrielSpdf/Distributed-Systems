package promotions

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/cryptography"
	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSubscribeIntegration(t *testing.T) {
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

	email := fmt.Sprintf("promotions-%d@example.com", time.Now().UnixNano())
	var userID string

	err = pool.QueryRow(ctx, `
		INSERT INTO gateway.users (name, email, password_hash)
		VALUES ('Teste promoções', $1, 'hash-apenas-para-teste')
		RETURNING id::text
	`, email).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}

	// Remove somente os dados criados por este teste.
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, query := range []string{
			"DELETE FROM gateway.promotion_outbox WHERE user_id = $1",
			"DELETE FROM gateway.promotion_preferences WHERE user_id = $1",
			"DELETE FROM gateway.users WHERE id = $1",
		} {
			if _, err := pool.Exec(cleanupCtx, query, userID); err != nil {
				t.Errorf("limpeza: %v", err)
			}
		}
	}()

	repository := NewRepository(pool)
	categories := []string{"limpeza", "alimentos"}

	changed, err := repository.Subscribe(ctx, userID, categories)
	if err != nil || !changed {
		t.Fatalf("primeira inscrição: alterado=%v, erro=%v", changed, err)
	}

	if !slices.Equal(categories, []string{"limpeza", "alimentos"}) {
		t.Fatal("Subscribe modificou a lista recebida")
	}

	assertCounts := func(wantPreferences, wantEvents int) {
		t.Helper()

		var preferences, events int
		err := pool.QueryRow(ctx, `
			SELECT
				(SELECT COUNT(*) FROM gateway.promotion_preferences
				 WHERE user_id = $1 AND active = TRUE),
				(SELECT COUNT(*) FROM gateway.promotion_outbox
				 WHERE user_id = $1)
		`, userID).Scan(&preferences, &events)
		if err != nil {
			t.Fatal(err)
		}
		if preferences != wantPreferences || events != wantEvents {
			t.Fatalf(
				"preferências=%d, eventos=%d; esperado=%d/%d",
				preferences, events, wantPreferences, wantEvents,
			)
		}
	}

	assertCounts(2, 1)

	activeCategories, err := repository.ListActive(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(activeCategories, []string{"alimentos", "limpeza"}) {
		t.Fatalf("categorias ativas incorretas: %v", activeCategories)
	}

	var payloadJSON []byte
	var status string
	err = pool.QueryRow(ctx, `
		SELECT payload, status
		FROM gateway.promotion_outbox
		WHERE user_id = $1
		ORDER BY sequence_id
		LIMIT 1
	`, userID).Scan(&payloadJSON, &status)
	if err != nil {
		t.Fatal(err)
	}

	var payload InterestPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}

	if status != "PENDENTE" ||
		payload.UserID != userID ||
		payload.Email != email ||
		payload.Action != ActionSubscribe ||
		payload.OccurredAt.IsZero() ||
		!slices.Equal(payload.Categories, []string{"alimentos", "limpeza"}) {
		t.Fatalf("evento incorreto: status=%s, payload=%+v", status, payload)
	}

	changed, err = repository.Subscribe(ctx, userID, categories)
	if err != nil || changed {
		t.Fatalf("inscrição repetida: alterado=%v, erro=%v", changed, err)
	}
	assertCounts(2, 1)

	changed, err = repository.Subscribe(ctx, userID, []string{"roupas"})
	if err == nil || changed {
		t.Fatalf("categoria inválida: alterado=%v, erro=%v", changed, err)
	}
	assertCounts(2, 1)

	changed, err = repository.Unsubscribe(ctx, userID, "limpeza")
	if err != nil || !changed {
		t.Fatalf("cancelamento: alterado=%v, erro=%v", changed, err)
	}
	assertCounts(1, 2)

	activeCategories, err = repository.ListActive(ctx, userID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(activeCategories, []string{"alimentos"}) {
		t.Fatalf("categorias após cancelamento: %v", activeCategories)
	}

	err = pool.QueryRow(ctx, `
		SELECT payload, status
		FROM gateway.promotion_outbox
		WHERE user_id = $1
		ORDER BY sequence_id DESC
		LIMIT 1
	`, userID).Scan(&payloadJSON, &status)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if status != "PENDENTE" ||
		payload.UserID != userID ||
		payload.Email != email ||
		payload.Action != ActionUnsubscribe ||
		payload.OccurredAt.IsZero() ||
		!slices.Equal(payload.Categories, []string{"limpeza"}) {
		t.Fatalf("evento de cancelamento incorreto: %+v", payload)
	}

	changed, err = repository.Unsubscribe(ctx, userID, "limpeza")
	if err != nil || changed {
		t.Fatalf("cancelamento repetido: alterado=%v, erro=%v", changed, err)
	}
	assertCounts(1, 2)

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	var outboxEventID string
	err = pool.QueryRow(ctx, `
		SELECT event_id::text
		FROM gateway.promotion_outbox
		WHERE user_id = $1
		ORDER BY sequence_id
		LIMIT 1
	`, userID).Scan(&outboxEventID)
	if err != nil {
		t.Fatal(err)
	}

	if err := repository.PrepareOutboxEnvelope(
		ctx, outboxEventID, privateKey,
	); err != nil {
		t.Fatal(err)
	}

	var firstEnvelope string
	err = pool.QueryRow(ctx, `
		SELECT envelope, status
		FROM gateway.promotion_outbox
		WHERE event_id = $1
	`, outboxEventID).Scan(&firstEnvelope, &status)
	if err != nil {
		t.Fatal(err)
	}
	if firstEnvelope == "" || status != "PENDENTE" {
		t.Fatalf("preparação incorreta: status=%s", status)
	}

	var savedEnvelope events.EventEnvelope
	if err := json.Unmarshal([]byte(firstEnvelope), &savedEnvelope); err != nil {
		t.Fatal(err)
	}
	if savedEnvelope.EventID != outboxEventID {
		t.Fatal("identificador da outbox não foi preservado")
	}

	registry := cryptography.PublicKeyRegistry{
		events.ProducerPrincipal: &privateKey.PublicKey,
	}
	if err := cryptography.VerifyEnvelopeFromProducer(
		savedEnvelope, registry,
	); err != nil {
		t.Fatal(err)
	}

	if err := repository.PrepareOutboxEnvelope(
		ctx, outboxEventID, privateKey,
	); err != nil {
		t.Fatal(err)
	}

	var secondEnvelope string
	err = pool.QueryRow(ctx, `
		SELECT envelope
		FROM gateway.promotion_outbox
		WHERE event_id = $1
	`, outboxEventID).Scan(&secondEnvelope)
	if err != nil {
		t.Fatal(err)
	}
	if secondEnvelope != firstEnvelope {
		t.Fatal("preparar novamente alterou o envelope assinado")
	}

	assertCounts(1, 2)

	t.Run("publicacao_da_outbox", func(t *testing.T) {
		var cancellationEventID string
		err := pool.QueryRow(ctx, `
			SELECT event_id::text
			FROM gateway.promotion_outbox
			WHERE user_id = $1
			ORDER BY sequence_id DESC
			LIMIT 1
		`, userID).Scan(&cancellationEventID)
		if err != nil {
			t.Fatal(err)
		}

		if err := repository.PrepareOutboxEnvelope(
			ctx, cancellationEventID, privateKey,
		); err != nil {
			t.Fatal(err)
		}

		// O cancelamento não pode ultrapassar a inscrição.
		handled, err := repository.PublishPreparedOutboxEvent(
			ctx, cancellationEventID,
			func(context.Context, []byte) error {
				t.Fatal("cancelamento publicado antes da inscrição")
				return nil
			},
		)
		if err != nil || handled {
			t.Fatalf("evento posterior: tratado=%v, erro=%v", handled, err)
		}

		// Simula falha no envio da inscrição.
		handled, err = repository.PublishPreparedOutboxEvent(
			ctx, outboxEventID,
			func(_ context.Context, data []byte) error {
				if string(data) != firstEnvelope {
					t.Fatal("envelope salvo não foi reutilizado")
				}
				return fmt.Errorf("falha simulada de publicação")
			},
		)
		if err == nil || !handled {
			t.Fatalf("falha simulada: tratado=%v, erro=%v", handled, err)
		}

		var attempts int
		var retryScheduled bool
		var lastError string
		err = pool.QueryRow(ctx, `
			SELECT status, attempt_count,
			       next_attempt_at > clock_timestamp(),
			       COALESCE(last_error, '')
			FROM gateway.promotion_outbox
			WHERE event_id = $1
		`, outboxEventID).Scan(
			&status, &attempts, &retryScheduled, &lastError,
		)
		if err != nil {
			t.Fatal(err)
		}
		if status != "PENDENTE" || attempts != 1 ||
			!retryScheduled || lastError == "" {
			t.Fatalf(
				"reagendamento incorreto: status=%s, tentativas=%d, erro=%s",
				status, attempts, lastError,
			)
		}

		// Antes do prazo, não deve chamar o publicador novamente.
		handled, err = repository.PublishPreparedOutboxEvent(
			ctx, outboxEventID,
			func(context.Context, []byte) error {
				t.Fatal("tentativa executada antes do prazo")
				return nil
			},
		)
		if err != nil || handled {
			t.Fatalf("antes do prazo: tratado=%v, erro=%v", handled, err)
		}

		// Antecipa apenas o evento deste teste, sem esperar.
		_, err = pool.Exec(ctx, `
			UPDATE gateway.promotion_outbox
			SET next_attempt_at = clock_timestamp() - INTERVAL '1 second'
			WHERE event_id = $1
		`, outboxEventID)
		if err != nil {
			t.Fatal(err)
		}

		handled, err = repository.PublishPreparedOutboxEvent(
			ctx, outboxEventID,
			func(_ context.Context, data []byte) error {
				if string(data) != firstEnvelope {
					t.Fatal("envelope mudou entre tentativas")
				}
				return nil
			},
		)
		if err != nil || !handled {
			t.Fatalf("publicação: tratado=%v, erro=%v", handled, err)
		}

		var publishedAtPresent bool
		err = pool.QueryRow(ctx, `
			SELECT status, attempt_count, published_at IS NOT NULL,
			       COALESCE(last_error, '')
			FROM gateway.promotion_outbox
			WHERE event_id = $1
		`, outboxEventID).Scan(
			&status, &attempts, &publishedAtPresent, &lastError,
		)
		if err != nil {
			t.Fatal(err)
		}
		if status != "PUBLICADO" || attempts != 2 ||
			!publishedAtPresent || lastError != "" {
			t.Fatalf("registro de publicação incorreto: status=%s", status)
		}

		// Após publicar a inscrição, o cancelamento pode avançar.
		handled, err = repository.PublishPreparedOutboxEvent(
			ctx, cancellationEventID,
			func(context.Context, []byte) error { return nil },
		)
		if err != nil || !handled {
			t.Fatalf("cancelamento: tratado=%v, erro=%v", handled, err)
		}
	})
}
