# Etapa 02 - Contratos de integração

> **Atualizado em 15/10/2026 (Etapa 17):** autoridade do prazo no MS Pagamento
> (seções 7 e 8), dados de envio no detalhe do pedido em vez de rota própria
> (seção 10), e-mail na inscrição de promoções (seção 11), método QUERY retirado
> (seção 14) e regras de falha e repetição (seção 18).

Este documento registra os contratos aprovados antes da implementação. O
objetivo é permitir que Lucas e Gabriel trabalhem em paralelo usando simulações
compatíveis com as interfaces reais.

## Responsáveis

- **Lucas:** React, API Gateway, autenticação, pedidos, HATEOAS, SSE e QUERY;
- **Gabriel:** Estoque, Pagamento, Mock, Entrega, Promoções, e-mail e topologia
  RabbitMQ;
- **Ambos:** contratos de eventos, assinatura digital, integração e testes.

## Convenções

- JSON utiliza `snake_case` nos eventos RabbitMQ e integrações internas;
- datas utilizam UTC no formato RFC 3339;
- identificadores são strings imutáveis;
- valores monetários usam duas casas decimais e moeda `BRL`;
- endpoints públicos exigem autenticação, exceto cadastro e login;
- o cliente autenticado é obtido pelo Gateway, nunca aceito livremente no corpo;
- erros HTTP seguem o mesmo formato;
- todas as mensagens RabbitMQ são assinadas com RSA-PSS e SHA-256.

Formato de erro:

```json
{
  "code": "INVALID_REQUEST",
  "message": "Descrição legível do erro"
}
```

---

## 1. Catálogo

### Gateway para Estoque

```http
GET /internal/products
```

Filtros opcionais:

```text
category
available
```

Exemplo:

```http
GET /internal/products?category=alimentos&available=true
```

Resposta:

```json
{
  "products": [
    {
      "id": "PROD-001",
      "name": "Arroz 5 kg",
      "category": "alimentos",
      "price": 29.90,
      "quantity": 30
    }
  ],
  "total": 1
}
```

### React para Gateway

```http
GET /api/products
```

O Gateway consulta o Estoque por REST e não acessa diretamente suas tabelas.
Se o Estoque estiver indisponível, o Gateway retorna `503 Service Unavailable`.

---

## 2. Envelope RabbitMQ

```json
{
  "event_id": "1a0ca180-3e62-4b45-984e-447287d1535e",
  "event_type": "pedido.criado",
  "producer": "ms-principal",
  "timestamp": "2026-10-01T18:00:00Z",
  "payload": {},
  "signature": "ASSINATURA_BASE64"
}
```

Validações obrigatórias do consumidor:

1. JSON válido;
2. routing key igual a `event_type`;
3. produtor autorizado para o tipo do evento;
4. chave pública conhecida;
5. assinatura válida;
6. payload válido;
7. idempotência da operação;
8. `Ack` somente depois da persistência.

---

## 3. Matriz de eventos

| Evento | Produtor | Consumidores |
|---|---|---|
| `pedido.criado` | Principal | Estoque |
| `pedido.excluido` | Principal | Estoque |
| `pedido.estoque_ok` | Estoque | Principal e Pagamento |
| `estoque.indisponivel` | Estoque | Principal |
| `pagamento.checkout_disponivel` | Pagamento | Principal |
| `pagamento.aprovado` | Pagamento | Principal e Entrega |
| `pagamento.recusado` | Pagamento | Principal |
| `pedido.enviado` | Entrega | Principal |
| `interesse.promocao` | Principal | Promoções |

Todos utilizam a exchange `eCommerce`, do tipo `direct`.

---

## 4. Evento `pedido.criado`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "items": [
    {
      "product_id": "PROD-001",
      "product_name": "Arroz 5 kg",
      "quantity": 2,
      "unit_price": 29.90
    }
  ],
  "total": 59.80,
  "currency": "BRL"
}
```

O Gateway persiste o pedido como `PENDENTE` antes de publicar. O Estoque cria
no máximo uma reserva por `order_id`.

---

## 5. Respostas do Estoque

### `pedido.estoque_ok`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "total": 59.80,
  "currency": "BRL",
  "reserved_at": "2026-10-01T18:01:00Z"
}
```

### `estoque.indisponivel`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "unavailable_items": [
    {
      "product_id": "PROD-001",
      "requested_quantity": 2,
      "available_quantity": 1
    }
  ],
  "reason": "Estoque insuficiente"
}
```

O Estoque verifica todos os itens antes da baixa. Não existe reserva parcial.
Resultados por pedido: `RESERVED`, `UNAVAILABLE` ou `RELEASED`.

---

## 6. Cobrança e checkout

### Pagamento para Mock

```http
POST /charges
Content-Type: application/json
Idempotency-Key: PED-001
```

```json
{
  "order_id": "PED-001",
  "amount": 59.80,
  "currency": "BRL",
  "webhook_url": "http://ms-pagamento:8082/webhooks/payments"
}
```

Resposta:

```json
{
  "charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "order_id": "PED-001",
  "status": "PENDING",
  "checkout_url": "http://localhost:8083/checkout/d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "expires_at": "2026-10-01T18:31:00Z"
}
```

### Evento `pagamento.checkout_disponivel`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "checkout_url": "http://localhost:8083/checkout/d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "expires_at": "2026-10-01T18:31:00Z"
}
```

O React abre a URL em uma nova aba.

---

## 7. Webhook

```http
POST /webhooks/payments
Content-Type: application/json
X-Webhook-Signature: sha256=ASSINATURA_HMAC
```

```json
{
  "event_id": "whk-5f550ca7-236d-4d79-9584-f5edbc49fce2",
  "charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "order_id": "PED-001",
  "status": "APPROVED",
  "occurred_at": "2026-10-01T18:10:00Z"
}
```

O Mock assina o corpo com HMAC-SHA256 e `PAYMENT_WEBHOOK_SECRET`. O Pagamento
registra `event_id` para impedir processamento duplicado.

Respostas principais:

- `204`: processado ou duplicado já processado;
- `400`: conteúdo inválido;
- `401`: assinatura inválida;
- `404`: cobrança inexistente;
- `409`: resultado conflitante, `event_id` já usado por outra cobrança
  (`EVENT_ID_CONFLICT`) ou decisão posterior ao prazo (`CHECKOUT_EXPIRED`).

O MS Pagamento é a autoridade do prazo de cada cobrança. A linha da cobrança é
travada na decisão: ou o webhook aprova/recusa antes, ou a expiração vence; as
duas nunca valem juntas. Um webhook com `occurred_at` depois de `expires_at`,
ou para uma cobrança já expirada, recebe `409` e não gera evento. O Mock trata
esse `409` como recusa definitiva da loja.

---

## 8. Resultado do pagamento

### `pagamento.aprovado`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "amount": 59.80,
  "currency": "BRL",
  "processed_at": "2026-10-01T18:10:00Z"
}
```

### `pagamento.recusado`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "charge_id": "d67af0cb-b7e8-44c7-a035-22c349c2cbc3",
  "amount": 59.80,
  "currency": "BRL",
  "reason": "Pagamento recusado pelo usuário",
  "processed_at": "2026-10-01T18:10:00Z"
}
```

Após recusa, o Principal publica `pedido.excluido` para liberar o estoque.

### Checkout expirado

Quando o prazo vence sem decisão (5 s de tolerância), o MS Pagamento marca a
cobrança como `EXPIRED` e publica `pagamento.recusado` com
`"reason": "CHECKOUT_EXPIRED"`. O Gateway coloca o pedido em `CANCELADO`
(não em `PAGAMENTO_RECUSADO`) e publica `pedido.excluido` com motivo
`CHECKOUT_EXPIRED`. O executor de expiração do Gateway fica como rede de
segurança: só cancela checkouts vencidos há mais de 10 minutos, caso o
Pagamento esteja fora do ar.

---

## 9. Cancelamento e compensação

### Endpoint

```http
DELETE /api/orders/{orderId}
```

Permitido em `PENDENTE` ou `ESTOQUE_CONFIRMADO`. Após checkout ou pagamento,
retorna `409 Conflict`.

### Evento `pedido.excluido`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "reason": "PAYMENT_REFUSED",
  "cancelled_at": "2026-10-01T18:11:00Z"
}
```

Motivos permitidos:

```text
CUSTOMER_CANCELLED
PAYMENT_REFUSED
CHECKOUT_EXPIRED
INTERNAL_FAILURE
```

Se ainda não houver reserva, o Estoque registra o cancelamento preventivo para
impedir uma reserva posterior.

---

## 10. Entrega

### Evento `pedido.enviado`

```json
{
  "order_id": "PED-001",
  "customer_id": "083a7808-462b-4df3-a755-3130bc571351",
  "invoice_id": "NF-PED-001",
  "tracking_code": "BRPED001BR",
  "shipped_at": "2026-10-01T18:12:00Z"
}
```

Cada `order_id` gera apenas uma entrega, nota fiscal e código de rastreamento.

Não há rota própria de rastreamento: depois do envio, `GET /api/orders` e
`GET /api/orders/{orderId}` incluem `invoice_id`, `tracking_code` e
`shipped_at`, e a tela de pedidos os mostra. (A rota
`/api/orders/{orderId}/tracking` foi retirada; o enunciado exige apenas a
notificação "Pedido Enviado".)

---

## 11. Promoções

### Endpoints públicos

```http
GET    /api/promotion-interests
POST   /api/promotion-interests
DELETE /api/promotion-interests/{category}
```

O cliente pode informar o e-mail na inscrição (enunciado: "registrar interesse,
informando o e-mail"):

```json
{ "categories": ["alimentos"], "email": "cliente@email.com" }
```

`email` é opcional; sem ele vale o e-mail já usado nas preferências ou, na
primeira inscrição, o da conta. E-mail inválido recebe `400`. Trocar só o
e-mail gera novo `interesse.promocao` com todas as categorias ativas.
`GET /api/promotion-interests` devolve `{"categories": [...], "email": "..."}`.

### Evento `interesse.promocao`

```json
{
  "user_id": "083a7808-462b-4df3-a755-3130bc571351",
  "email": "cliente@email.com",
  "action": "SUBSCRIBE",
  "categories": ["alimentos", "eletronicos"],
  "occurred_at": "2026-10-01T19:00:00Z"
}
```

A ação pode ser `SUBSCRIBE` ou `UNSUBSCRIBE`. A combinação `user_id` e
`category` é única.

O MS Promoções consulta produtos do Estoque, gera descontos, localiza clientes
interessados, envia os e-mails e registra cada tentativa como `PENDING`, `SENT`
ou `FAILED`.

---

## 12. SSE

Endpoint autenticado:

```http
GET /api/orders/events
```

Cabeçalhos:

```http
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

Exemplo:

```text
event: order.status.changed
id: EVENT_ID
data: {"orderId":"PED-001","status":"PAGAMENTO_APROVADO"}

```

Cada conexão é associada ao usuário autenticado. O Gateway envia somente os
eventos dos pedidos daquele usuário. Após reconectar, o React consulta
`GET /api/orders` para sincronizar o estado atual.

Eventos SSE:

```text
connection.ready
order.status.changed
payment.checkout.available
```

---

## 13. HATEOAS

As respostas de pedidos incluem somente as ações válidas para o estado atual.

Exemplo:

```json
{
  "id": "PED-001",
  "status": "AGUARDANDO_PAGAMENTO",
  "_links": {
    "self": {
      "href": "/api/orders/PED-001",
      "method": "GET"
    },
    "checkout": {
      "href": "http://localhost:8083/checkout/CHARGE_ID",
      "method": "GET"
    }
  }
}
```

Links são relativos quando pertencem ao Gateway. O frontend respeita os links
retornados e não presume que uma ação está sempre disponível.

---

## 14. Método QUERY (retirado)

O endpoint `QUERY /api/orders/search` não foi implementado e saiu do contrato:
o enunciado não exige busca nem o método QUERY. A listagem é feita por
`GET /api/orders`.

---

## 15. Estados do pedido

```text
PENDENTE
├── CANCELADO
├── ESTOQUE_INDISPONIVEL
└── ESTOQUE_CONFIRMADO
    ├── CANCELADO
    └── AGUARDANDO_PAGAMENTO
        ├── CANCELADO (checkout expirado)
        ├── PAGAMENTO_RECUSADO
        └── PAGAMENTO_APROVADO
            └── ENVIADO
```

Estados finais:

```text
CANCELADO
ESTOQUE_INDISPONIVEL
PAGAMENTO_RECUSADO
ENVIADO
FALHA_NO_PROCESSAMENTO
```

---

## 16. Propriedade dos dados

| Schema | Responsável |
|---|---|
| `gateway` | API Gateway |
| `estoque` | MS Estoque |
| `pagamento` | MS Pagamento |
| `entrega` | MS Entrega |
| `promocoes` | MS Promoções |

Nenhum serviço acessa diretamente as tabelas de outro. A comunicação acontece
por REST ou RabbitMQ.

---

## 17. Estratégia de desenvolvimento paralelo

### Lucas pode simular

- catálogo do Estoque;
- eventos de estoque;
- checkout disponível;
- pagamento aprovado ou recusado;
- pedido enviado.

### Gabriel pode simular

- publicação de `pedido.criado`;
- publicação de `pedido.excluido`;
- chamadas ao Mock;
- envio de webhook;
- API externa de e-mail.

As simulações devem usar exatamente os contratos deste documento.

## 18. Falhas, repetição e revisão

Estoque, Pagamento e Entrega seguem as mesmas regras:

- cada evento de saída é assinado uma vez e gravado no banco antes de ser
  publicado; toda repetição reenvia o mesmo `event_id`;
- a publicação usa confirmação do RabbitMQ e `mandatory`; o evento só é marcado
  como publicado depois da confirmação, e uma rotina reenvia os pendentes a
  cada 10 s;
- mensagem inválida (JSON, routing key, produtor, assinatura ou payload) é
  rejeitada e registrada, sem novas tentativas;
- falha temporária (banco, Mock, RabbitMQ) tem até 5 tentativas com espera de
  1, 2, 4 e 8 s; esgotadas, a mensagem original vai para a fila
  `<fila>.revisao` (`estoque.revisao`, `pagamento.revisao`,
  `entrega.revisao`) com motivo, tentativas e routing key original;
- `go run ./cmd/reprocessar -fila <fila>` devolve as mensagens da revisão para
  a fila original depois de resolvida a causa.

O Gateway e o MS Promoções mantêm seus próprios inbox/outbox.

---

## Critério de conclusão

- todos os integrantes aprovam os contratos;
- cada evento possui produtor e consumidores definidos;
- REST, webhook, SSE e QUERY possuem exemplos;
- estados e transições estão definidos;
- propriedade dos dados está definida;
- Lucas e Gabriel conseguem desenvolver sem acessar o banco ou código interno
  um do outro.
