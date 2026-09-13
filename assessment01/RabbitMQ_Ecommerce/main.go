package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"RabbitMQ_Ecommerce/utils/events"
	"RabbitMQ_Ecommerce/utils/misc"
	"RabbitMQ_Ecommerce/utils/rabbitmq"
)

// testData espelha as seções de testdata/events.json usadas por este publicador.
type testData struct {
	OrderCreatedPayload     events.OrderCreatedPayload     `json:"order_created_payload"`
	OrderReferencePayload   events.OrderReferencePayload   `json:"order_reference_payload"`
	StockUnavailablePayload events.StockUnavailablePayload `json:"stock_unavailable_payload"`
	PaymentApprovedPayload  events.PaymentResultPayload    `json:"payment_approved_payload"`
	PaymentRefusedPayload   events.PaymentResultPayload    `json:"payment_refused_payload"`
	OrderShippedPayload     events.OrderShippedPayload     `json:"order_shipped_payload"`
	PromotionCategoryA      events.PromotionPayload        `json:"promotion_payload_category_a"`
	PromotionCategoryB      events.PromotionPayload        `json:"promotion_payload_category_b"`
	PromotionCategoryC      events.PromotionPayload        `json:"promotion_payload_category_c"`
}

var validActions = []string{
	"create",
	"remove",
	"estoque-ok",
	"estoque-indisponivel",
	"pagamento-aprovado",
	"pagamento-recusado",
	"enviado",
	"promocao-a",
	"promocao-b",
	"promocao-c",
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf(
			"uso: go run . <%v> <order_id>",
			validActions,
		)
	}

	action := os.Args[1]
	orderID := os.Args[2]

	if orderID == "" {
		return fmt.Errorf("identificador do pedido não pode ser vazio")
	}

	data, err := loadTestData("testdata/events.json")
	if err != nil {
		return err
	}

	exchangeName := events.ExchangeEcommerce

	var (
		eventType string
		payload   any
	)

	switch action {
	case "create":
		p := data.OrderCreatedPayload
		p.OrderID = orderID
		eventType = events.PedidoCriado
		payload = p

	case "remove":
		p := data.OrderReferencePayload
		p.OrderID = orderID
		eventType = events.PedidoExcluido
		payload = p

	case "estoque-ok":
		// pedido.estoque_ok só precisa identificar o pedido, por isso reaproveita
		// OrderReferencePayload usa o mesmo formato usado em pedido.excluido.
		p := data.OrderReferencePayload
		p.OrderID = orderID
		eventType = events.PedidoEstoqueOk
		payload = p

	case "estoque-indisponivel":
		p := data.StockUnavailablePayload
		p.OrderID = orderID
		eventType = events.EstoqueIndisponivel
		payload = p

	case "pagamento-aprovado":
		p := data.PaymentApprovedPayload
		p.OrderID = orderID
		eventType = events.PagamentoAprovado
		payload = p

	case "pagamento-recusado":
		p := data.PaymentRefusedPayload
		p.OrderID = orderID
		eventType = events.PagamentoRecusado
		payload = p

	case "enviado":
		p := data.OrderShippedPayload
		p.OrderID = orderID
		eventType = events.PedidoEnviado
		payload = p

	case "promocao-a", "promocao-b", "promocao-c":
		// Promoções nao sao referentes a um pedido
		// o <order_id> do CLI eh ignorado aqui e o payload vem pronto de testdata/events.json
		exchangeName = events.ExchangePromocoes

		promotions := map[string]events.PromotionPayload{
			"promocao-a": data.PromotionCategoryA,
			"promocao-b": data.PromotionCategoryB,
			"promocao-c": data.PromotionCategoryC,
		}
		routingKeys := map[string]string{
			"promocao-a": events.PromocaoCategoriaA,
			"promocao-b": events.PromocaoCategoriaB,
			"promocao-c": events.PromocaoCategoriaC,
		}

		eventType = routingKeys[action]
		payload = promotions[action]

	default:
		return fmt.Errorf(
			"ação inválida: %s (use uma de %v)",
			action,
			validActions,
		)
	}

	envelope, err := misc.MountEnvelope(payload, eventType, "EcommerceService")
	if err != nil {
		return err
	}

	connection, err := rabbitmq.Connect()
	if err != nil {
		return err
	}
	defer connection.Close()

	channel, err := rabbitmq.OpenChannel(connection)
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := rabbitmq.DeclareExchanges(channel); err != nil {
		return err
	}

	if err := rabbitmq.PublishEvent(
		channel,
		exchangeName,
		eventType,
		envelope,
	); err != nil {
		return err
	}

	log.Printf(
		"[✓] Publicação enviada: evento=%s tipo=%s pedido=%s",
		envelope.EventID,
		envelope.EventType,
		orderID,
	)

	return nil
}

// loadTestData abre e decodifica os exemplos usados para montar publicações manuais
func loadTestData(path string) (*testData, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("erro ao abrir dados de teste: %w", err)
	}
	defer file.Close()

	var data testData
	if err := json.NewDecoder(file).Decode(&data); err != nil {
		return nil, fmt.Errorf("erro ao decodificar dados de teste: %w", err)
	}

	return &data, nil
}
