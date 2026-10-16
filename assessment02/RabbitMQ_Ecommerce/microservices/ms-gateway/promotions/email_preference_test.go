package promotions

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNormalizeEmail(t *testing.T) {
	valid := map[string]string{
		"cliente@example.com":       "cliente@example.com",
		"  Cliente@Example.COM  ":   "cliente@example.com",
		"nome.sobrenome@empresa.br": "nome.sobrenome@empresa.br",
	}

	for input, expected := range valid {
		got, err := normalizeEmail(input)
		if err != nil || got != expected {
			t.Fatalf("%q: esperava %q, recebeu %q (%v)", input, expected, got, err)
		}
	}

	invalid := []string{
		"sem-arroba",
		"Nome <cliente@example.com>",
		"cliente@",
		"@example.com",
		"cliente@example.com, outro@example.com",
		strings.Repeat("a", 250) + "@example.com",
	}

	for _, input := range invalid {
		if _, err := normalizeEmail(input); err == nil {
			t.Fatalf("%q deveria ser inválido", input)
		}
	}
}

func TestSubscribeWithEmailIntegration(t *testing.T) {
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

	accountEmail := fmt.Sprintf("conta-%d@example.com", time.Now().UnixNano())
	var userID string

	if err := pool.QueryRow(ctx, `
		INSERT INTO gateway.users (name, email, password_hash)
		VALUES ('Teste e-mail de promoções', $1, 'hash-apenas-para-teste')
		RETURNING id::text
	`, accountEmail).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
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

	lastEvent := func() InterestPayload {
		t.Helper()

		var data []byte
		if err := pool.QueryRow(ctx, `
			SELECT payload FROM gateway.promotion_outbox
			WHERE user_id = $1 ORDER BY sequence_id DESC LIMIT 1
		`, userID).Scan(&data); err != nil {
			t.Fatal(err)
		}

		var payload InterestPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}

		return payload
	}

	countEvents := func() int {
		t.Helper()

		var count int
		if err := pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM gateway.promotion_outbox WHERE user_id = $1", userID,
		).Scan(&count); err != nil {
			t.Fatal(err)
		}

		return count
	}

	preferred, err := repository.PreferredEmail(ctx, userID)
	if err != nil || preferred != accountEmail {
		t.Fatalf("sem preferência, esperava o e-mail da conta: %q %v", preferred, err)
	}

	changed, err := repository.SubscribeWithEmail(ctx, userID, []string{"alimentos"}, "demo@example.com")
	if err != nil || !changed {
		t.Fatalf("inscrição com e-mail: alterou=%v erro=%v", changed, err)
	}
	if event := lastEvent(); event.Email != "demo@example.com" || !slices.Equal(event.Categories, []string{"alimentos"}) {
		t.Fatalf("evento com e-mail inesperado: %+v", event)
	}

	preferred, _ = repository.PreferredEmail(ctx, userID)
	if preferred != "demo@example.com" {
		t.Fatalf("e-mail escolhido não ficou como preferência: %q", preferred)
	}

	changed, err = repository.SubscribeWithEmail(ctx, userID, []string{"alimentos"}, "demo@example.com")
	if err != nil || changed {
		t.Fatalf("mesma inscrição não deveria gerar evento: alterou=%v erro=%v", changed, err)
	}

	changed, err = repository.SubscribeWithEmail(ctx, userID, []string{"limpeza"}, "demo@example.com")
	if err != nil || !changed || !slices.Equal(lastEvent().Categories, []string{"limpeza"}) {
		t.Fatalf("nova categoria: alterou=%v erro=%v evento=%+v", changed, err, lastEvent())
	}

	before := countEvents()
	changed, err = repository.SubscribeWithEmail(ctx, userID, []string{"limpeza"}, "novo@example.com")
	if err != nil || !changed || countEvents() != before+1 {
		t.Fatalf("troca só do e-mail deveria gerar um evento: alterou=%v erro=%v", changed, err)
	}
	if event := lastEvent(); event.Email != "novo@example.com" ||
		!slices.Equal(event.Categories, []string{"alimentos", "limpeza"}) {
		t.Fatalf("troca de e-mail deveria atualizar todas as categorias ativas: %+v", event)
	}

	changed, err = repository.SubscribeWithEmail(ctx, userID, []string{"limpeza"}, "")
	if err != nil || changed {
		t.Fatalf("sem e-mail, a preferência atual deveria ser mantida: alterou=%v erro=%v", changed, err)
	}
}
