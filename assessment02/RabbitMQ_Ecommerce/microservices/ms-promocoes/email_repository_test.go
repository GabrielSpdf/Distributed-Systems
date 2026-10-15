package mspromocoes

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/events"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestQueuePromotionEmailsIntegration(t *testing.T) {
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

	var promotionEventID string
	err = pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").
		Scan(&promotionEventID)
	if err != nil {
		t.Fatal(err)
	}

	recipient := fmt.Sprintf("promo-%s@example.com", promotionEventID)
	from := "Event Market <onboarding@resend.dev>"
	var userIDs []string

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(), 5*time.Second,
		)
		defer cleanupCancel()

		for _, userID := range userIDs {
			for _, query := range []string{
				"DELETE FROM promocoes.email_logs WHERE user_id = $1",
				"DELETE FROM promocoes.email_jobs WHERE user_id = $1",
				"DELETE FROM promocoes.promotion_interests WHERE user_id = $1",
			} {
				if _, err := pool.Exec(cleanupCtx, query, userID); err != nil {
					t.Errorf("erro na limpeza: %v", err)
				}
			}
		}
	}()

	fixtures := []struct {
		category string
		active   bool
		email    string
	}{
		{"limpeza", true, recipient},
		{"limpeza", false, recipient},
		{"alimentos", true, recipient},
		{"limpeza", true, "outro-" + recipient},
	}

	for _, fixture := range fixtures {
		var userID string
		if err := pool.QueryRow(ctx, "SELECT gen_random_uuid()::text").
			Scan(&userID); err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, userID)

		_, err := pool.Exec(ctx, `
			INSERT INTO promocoes.promotion_interests
				(user_id, email, category, active)
			VALUES ($1, $2, $3, $4)
		`, userID, fixture.email, fixture.category, fixture.active)
		if err != nil {
			t.Fatal(err)
		}
	}

	promotion := events.PromotionPayload{
		ProductID:          "PROD-002",
		ProductName:        "Detergente",
		Category:           "Limpeza",
		DiscountPercentage: 10,
		OriginalPrice:      10,
		PromotionalPrice:   9,
	}

	repository := NewEmailRepository(pool)
	created, err := repository.QueuePromotionEmails(
		ctx, promotionEventID, promotion, from, recipient,
	)
	if err != nil || created != 1 {
		t.Fatalf("envios criados=%d, erro=%v; esperado=1", created, err)
	}

	// Repetir a mesma oferta não deve alterar o conteúdo já salvo.
	promotion.ProductName = "Nome alterado após a primeira gravação"
	created, err = repository.QueuePromotionEmails(
		ctx, promotionEventID, promotion, from, recipient,
	)
	if err != nil || created != 0 {
		t.Fatalf("repetição: criados=%d, erro=%v", created, err)
	}

	var count int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM promocoes.email_jobs
		WHERE promotion_event_id = $1
	`, promotionEventID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("esperava um envio, encontrei %d", count)
	}

	var savedUserID, savedRecipient, savedFrom string
	var category, subject, textBody, status, idempotencyKey string
	var attempts int

	err = pool.QueryRow(ctx, `
		SELECT user_id::text, recipient, from_address,
		       category, subject, text_body, status,
		       idempotency_key, attempt_count
		FROM promocoes.email_jobs
		WHERE promotion_event_id = $1
	`, promotionEventID).Scan(
		&savedUserID, &savedRecipient, &savedFrom,
		&category, &subject, &textBody, &status,
		&idempotencyKey, &attempts,
	)
	if err != nil {
		t.Fatal(err)
	}

	if savedUserID != userIDs[0] ||
		savedRecipient != recipient ||
		savedFrom != from ||
		category != "limpeza" ||
		subject != "Oferta Event Market: Detergente" ||
		status != "PENDING" ||
		attempts != 0 ||
		idempotencyKey != "promotion/"+promotionEventID+"/"+userIDs[0] {
		t.Fatal("envio pendente foi salvo com dados incorretos")
	}

	originalPromotion := promotion
	originalPromotion.ProductName = "Detergente"
	expectedMessage, err := BuildPromotionEmail(originalPromotion, recipient)
	if err != nil {
		t.Fatal(err)
	}
	if textBody != expectedMessage.Text {
		t.Fatal("conteúdo original não foi preservado")
	}

	t.Run("preparacao_do_envio", func(t *testing.T) {
		var jobID string
		err := pool.QueryRow(ctx, `
			SELECT id::text
			FROM promocoes.email_jobs
			WHERE promotion_event_id = $1
		`, promotionEventID).Scan(&jobID)
		if err != nil {
			t.Fatal(err)
		}

		prepared, err := repository.PrepareEmailJob(ctx, jobID)
		if err != nil || !prepared {
			t.Fatalf("preparação: preparado=%v, erro=%v", prepared, err)
		}

		var firstAttempt time.Time
		err = pool.QueryRow(ctx, `
			SELECT first_attempt_at
			FROM promocoes.email_jobs
			WHERE id = $1
		`, jobID).Scan(&firstAttempt)
		if err != nil {
			t.Fatal(err)
		}

		prepared, err = repository.PrepareEmailJob(ctx, jobID)
		if err != nil || !prepared {
			t.Fatalf("nova preparação: preparado=%v, erro=%v", prepared, err)
		}

		var repeatedFirstAttempt time.Time
		err = pool.QueryRow(ctx, `
			SELECT first_attempt_at, status, attempt_count
			FROM promocoes.email_jobs
			WHERE id = $1
		`, jobID).Scan(&repeatedFirstAttempt, &status, &attempts)
		if err != nil {
			t.Fatal(err)
		}

		if firstAttempt.IsZero() ||
			!firstAttempt.Equal(repeatedFirstAttempt) ||
			status != "PENDING" || attempts != 0 {
			t.Fatal("a preparação alterou indevidamente o envio")
		}
	})

	t.Run("processamento_do_envio", func(t *testing.T) {
		var jobID string
		if err := pool.QueryRow(ctx, `
			SELECT id::text
			FROM promocoes.email_jobs
			WHERE promotion_event_id = $1
		`, promotionEventID).Scan(&jobID); err != nil {
			t.Fatal(err)
		}

		calls := 0
		send := func(
			_ context.Context,
			message EmailMessage,
			key string,
		) (string, error) {
			calls++

			if message != expectedMessage {
				t.Errorf("conteúdo do envio foi alterado: %+v", message)
			}
			if key != idempotencyKey {
				t.Errorf("chave de idempotência foi alterada: %q", key)
			}

			if calls == 1 {
				return "", &ResendHTTPError{StatusCode: 503}
			}
			return "provider-test-001", nil
		}

		handled, err := repository.ProcessEmailJob(
			ctx, jobID, from, recipient, send,
		)
		if !handled || err == nil || calls != 1 {
			t.Fatalf(
				"falha temporária: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}

		var jobStatus string
		var attemptCount int
		var retryScheduled bool

		if err := pool.QueryRow(ctx, `
			SELECT status, attempt_count,
				next_attempt_at > clock_timestamp()
			FROM promocoes.email_jobs
			WHERE id = $1
		`, jobID).Scan(
			&jobStatus, &attemptCount, &retryScheduled,
		); err != nil {
			t.Fatal(err)
		}

		if jobStatus != "PENDING" || attemptCount != 1 || !retryScheduled {
			t.Fatalf(
				"reagendamento incorreto: status=%s, tentativas=%d, futuro=%v",
				jobStatus, attemptCount, retryScheduled,
			)
		}

		// Ainda não chegou o horário da próxima tentativa.
		handled, err = repository.ProcessEmailJob(
			ctx, jobID, from, recipient, send,
		)
		if handled || err != nil || calls != 1 {
			t.Fatalf(
				"tentativa antecipada: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}

		// Avança apenas o envio deste teste, sem precisar aguardar.
		if _, err := pool.Exec(ctx, `
			UPDATE promocoes.email_jobs
			SET next_attempt_at = clock_timestamp() - INTERVAL '1 second'
			WHERE id = $1
		`, jobID); err != nil {
			t.Fatal(err)
		}

		handled, err = repository.ProcessEmailJob(
			ctx, jobID, from, recipient, send,
		)
		if !handled || err != nil || calls != 2 {
			t.Fatalf(
				"sucesso: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}

		var providerID string
		var sentAtPresent, errorCleared bool

		if err := pool.QueryRow(ctx, `
			SELECT status, attempt_count, provider_message_id,
				sent_at IS NOT NULL, last_error IS NULL
			FROM promocoes.email_jobs
			WHERE id = $1
		`, jobID).Scan(
			&jobStatus, &attemptCount, &providerID,
			&sentAtPresent, &errorCleared,
		); err != nil {
			t.Fatal(err)
		}

		if jobStatus != "SENT" || attemptCount != 2 ||
			providerID != "provider-test-001" ||
			!sentAtPresent || !errorCleared {
			t.Fatalf(
				"resultado incorreto: status=%s, tentativas=%d, provider=%q",
				jobStatus, attemptCount, providerID,
			)
		}

		// Um envio concluído não pode chamar o provedor novamente.
		handled, err = repository.ProcessEmailJob(
			ctx, jobID, from, recipient, send,
		)
		if handled || err != nil || calls != 2 {
			t.Fatalf(
				"envio duplicado: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}

		var totalLogs, failedLogs, sentLogs int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*),
				COUNT(*) FILTER (WHERE status = 'FAILED'),
				COUNT(*) FILTER (WHERE status = 'SENT')
			FROM promocoes.email_logs
			WHERE job_id = $1
		`, jobID).Scan(&totalLogs, &failedLogs, &sentLogs); err != nil {
			t.Fatal(err)
		}

		if totalLogs != 2 || failedLogs != 1 || sentLogs != 1 {
			t.Fatalf(
				"logs incorretos: total=%d, falhas=%d, sucessos=%d",
				totalLogs, failedLogs, sentLogs,
			)
		}
	})

	t.Run("bloqueios_de_seguranca", func(t *testing.T) {
		cases := []struct {
			name          string
			active        bool
			expired       bool
			attempts      int
			changedConfig bool
			wantStatus    string
		}{
			{"interesse_cancelado", false, false, 0, false, "CANCELLED"},
			{"janela_expirada", true, true, 0, false, "PENDING_REVIEW"},
			{"limite_de_tentativas", true, false, 12, false, "PENDING_REVIEW"},
			{"configuracao_alterada", true, false, 0, true, "PENDING_REVIEW"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				// Cada cenário começa com interesse ativo e uma oferta nova.
				if _, err := pool.Exec(ctx, `
					UPDATE promocoes.promotion_interests
					SET active = TRUE
					WHERE user_id = $1 AND category = 'limpeza'
				`, userIDs[0]); err != nil {
					t.Fatal(err)
				}

				var eventID string
				if err := pool.QueryRow(ctx,
					"SELECT gen_random_uuid()::text",
				).Scan(&eventID); err != nil {
					t.Fatal(err)
				}

				created, err := repository.QueuePromotionEmails(
					ctx, eventID, originalPromotion, from, recipient,
				)
				if err != nil || created != 1 {
					t.Fatalf("criados=%d, erro=%v", created, err)
				}

				var jobID string
				if err := pool.QueryRow(ctx, `
					UPDATE promocoes.email_jobs
					SET attempt_count = $2,
						first_attempt_at = CASE
							WHEN $3::boolean
							THEN clock_timestamp() - INTERVAL '24 hours'
							ELSE clock_timestamp()
						END
					WHERE promotion_event_id = $1
					RETURNING id::text
				`, eventID, tc.attempts, tc.expired).Scan(&jobID); err != nil {
					t.Fatal(err)
				}

				if _, err := pool.Exec(ctx, `
					UPDATE promocoes.promotion_interests
					SET active = $2
					WHERE user_id = $1 AND category = 'limpeza'
				`, userIDs[0], tc.active); err != nil {
					t.Fatal(err)
				}

				configuredFrom := from
				if tc.changedConfig {
					configuredFrom = "Outro remetente <outro@example.com>"
				}

				calls := 0
				handled, err := repository.ProcessEmailJob(
					ctx, jobID, configuredFrom, recipient,
					func(
						context.Context, EmailMessage, string,
					) (string, error) {
						calls++
						return "nao-deveria-enviar", nil
					},
				)
				if !handled || err != nil || calls != 0 {
					t.Fatalf(
						"handled=%v, erro=%v, chamadas=%d",
						handled, err, calls,
					)
				}

				var savedStatus string
				var savedAttempts, logs int
				if err := pool.QueryRow(ctx, `
					SELECT status, attempt_count,
						(SELECT COUNT(*)
							FROM promocoes.email_logs
							WHERE job_id = j.id)
					FROM promocoes.email_jobs j
					WHERE j.id = $1
				`, jobID).Scan(
					&savedStatus, &savedAttempts, &logs,
				); err != nil {
					t.Fatal(err)
				}

				if savedStatus != tc.wantStatus ||
					savedAttempts != tc.attempts || logs != 0 {
					t.Fatalf(
						"status=%s, tentativas=%d, logs=%d",
						savedStatus, savedAttempts, logs,
					)
				}
			})
		}
	})

	t.Run("busca_do_proximo_envio", func(t *testing.T) {
		// A busca é global: não devemos processar dados de outros testes.
		var pending int
		if err := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM promocoes.email_jobs
			WHERE status = 'PENDING'
		`).Scan(&pending); err != nil {
			t.Fatal(err)
		}
		if pending != 0 {
			t.Skip("existem pendências de outros testes no banco isolado")
		}

		var eventID string
		if err := pool.QueryRow(ctx,
			"SELECT gen_random_uuid()::text",
		).Scan(&eventID); err != nil {
			t.Fatal(err)
		}

		created, err := repository.QueuePromotionEmails(
			ctx, eventID, originalPromotion, from, recipient,
		)
		if err != nil || created != 1 {
			t.Fatalf("criados=%d, erro=%v", created, err)
		}

		var jobID string
		if err := pool.QueryRow(ctx, `
			UPDATE promocoes.email_jobs
			SET next_attempt_at = clock_timestamp() + INTERVAL '1 hour'
			WHERE promotion_event_id = $1
			RETURNING id::text
		`, eventID).Scan(&jobID); err != nil {
			t.Fatal(err)
		}

		calls := 0
		send := func(
			_ context.Context,
			message EmailMessage,
			key string,
		) (string, error) {
			calls++
			if message != expectedMessage {
				t.Error("conteúdo diferente do esperado")
			}
			expectedKey := "promotion/" + eventID + "/" + userIDs[0]
			if key != expectedKey {
				t.Errorf("chave inesperada: %q", key)
			}
			return "provider-worker-test", nil
		}

		// O envio ainda está agendado para o futuro.
		handled, err := repository.ProcessNextEmailJob(
			ctx, from, recipient, send,
		)
		if handled || err != nil || calls != 0 {
			t.Fatalf(
				"envio antecipado: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}

		if _, err := pool.Exec(ctx, `
			UPDATE promocoes.email_jobs
			SET next_attempt_at = clock_timestamp() - INTERVAL '1 second'
			WHERE id = $1
		`, jobID); err != nil {
			t.Fatal(err)
		}

		handled, err = repository.ProcessNextEmailJob(
			ctx, from, recipient, send,
		)
		if !handled || err != nil || calls != 1 {
			t.Fatalf(
				"processamento: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}

		var savedStatus string
		if err := pool.QueryRow(ctx, `
			SELECT status FROM promocoes.email_jobs WHERE id = $1
		`, jobID).Scan(&savedStatus); err != nil {
			t.Fatal(err)
		}
		if savedStatus != "SENT" {
			t.Fatalf("status=%s; esperado SENT", savedStatus)
		}

		// Não deve encontrar novamente o envio concluído.
		handled, err = repository.ProcessNextEmailJob(
			ctx, from, recipient, send,
		)
		if handled || err != nil || calls != 1 {
			t.Fatalf(
				"repetição: handled=%v, erro=%v, chamadas=%d",
				handled, err, calls,
			)
		}
	})
}
