package msentrega

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"RabbitMQ_Ecommerce/utils/database"
)

func openTestRepository(t *testing.T) (*Repository, string) {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL não definida; teste de integração ignorado")
	}

	pool, err := database.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("erro ao conectar ao banco de teste: %v", err)
	}
	t.Cleanup(pool.Close)

	prefix := "TEST-ENT-" + strconv.FormatInt(time.Now().UnixNano(), 36) + "-"

	t.Cleanup(func() {
		_, _ = pool.Exec(
			context.Background(),
			"DELETE FROM entrega.shipments WHERE order_id LIKE $1",
			prefix+"%",
		)
	})

	return NewRepository(pool), prefix
}

func testShipment(orderID string, suffix string) Shipment {
	return Shipment{
		OrderID:      orderID,
		CustomerID:   "083a7808-462b-4df3-a755-3130bc571351",
		ChargeID:     "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
		InvoiceID:    "NF-" + orderID + suffix,
		TrackingCode: "BR" + orderID + suffix,
		ShippedAt:    time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC),
		Envelope:     `{"event_id":"` + orderID + suffix + `"}`,
	}
}

func TestCreateShipmentKeepsOneShipmentPerOrder(t *testing.T) {
	repository, prefix := openTestRepository(t)
	ctx := context.Background()
	orderID := prefix + "1"

	first, err := repository.CreateShipment(ctx, testShipment(orderID, "-a"))
	if err != nil {
		t.Fatalf("erro ao criar entrega: %v", err)
	}

	if first.InvoiceID != "NF-"+orderID+"-a" || first.Published ||
		!first.ShippedAt.Equal(time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("entrega inesperada: %+v", first)
	}

	second, err := repository.CreateShipment(ctx, testShipment(orderID, "-b"))
	if err != nil {
		t.Fatalf("erro na segunda criação: %v", err)
	}

	if second.InvoiceID != first.InvoiceID || second.Envelope != first.Envelope {
		t.Fatalf("segunda aprovação criou outra entrega: %+v", second)
	}

	found, exists, err := repository.FindShipment(ctx, orderID)
	if err != nil || !exists || found.TrackingCode != first.TrackingCode {
		t.Fatalf("entrega não encontrada: %+v %v %v", found, exists, err)
	}

	_, exists, err = repository.FindShipment(ctx, prefix+"inexistente")
	if err != nil || exists {
		t.Fatalf("esperava ausência: %v %v", exists, err)
	}
}

func TestPendingShipmentsUntilMarkedPublished(t *testing.T) {
	repository, prefix := openTestRepository(t)
	ctx := context.Background()
	orderID := prefix + "1"

	if _, err := repository.CreateShipment(ctx, testShipment(orderID, "-a")); err != nil {
		t.Fatalf("erro ao criar entrega: %v", err)
	}

	isPending := func() bool {
		pending, err := repository.ListPendingShipments(ctx, 0, 100)
		if err != nil {
			t.Fatalf("erro ao listar pendências: %v", err)
		}

		for _, shipment := range pending {
			if shipment.OrderID == orderID {
				return true
			}
		}

		return false
	}

	if !isPending() {
		t.Fatal("entrega não publicada não aparece como pendente")
	}

	if err := repository.MarkPublished(ctx, orderID); err != nil {
		t.Fatalf("erro ao marcar publicação: %v", err)
	}

	if isPending() {
		t.Fatal("entrega publicada continua pendente")
	}

	found, _, _ := repository.FindShipment(ctx, orderID)
	if !found.Published {
		t.Fatal("publicação não foi gravada")
	}
}

func TestPendingShipmentsRespectMinimumAge(t *testing.T) {
	repository, prefix := openTestRepository(t)
	ctx := context.Background()
	orderID := prefix + "1"

	if _, err := repository.CreateShipment(ctx, testShipment(orderID, "-a")); err != nil {
		t.Fatalf("erro ao criar entrega: %v", err)
	}

	pending, err := repository.ListPendingShipments(ctx, time.Hour, 100)
	if err != nil {
		t.Fatalf("erro ao listar pendências: %v", err)
	}

	for _, shipment := range pending {
		if shipment.OrderID == orderID {
			t.Fatal("entrega recém-criada não deve ser reenviada pela rotina de pendências")
		}
	}
}
