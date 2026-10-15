# Etapa 14 - MS Pagamento e Mock de Pagamento

## Base e escopo

Branch `feat/estoque-pagamento`, junto com o MS Estoque (Etapa 13), pois usa
o Estoque real e a configuração daquela etapa. Gabriel substitui o Pagamento
aleatório da Avaliação 01 pelo fluxo da Avaliação 02: cobrança no Mock de
Pagamento, checkout em nova aba e retorno por webhook assinado com HMAC. A
tela de checkout que o React abre é a página do Mock.

Implementado e validado com o Gateway antes da revisão de escopo registrada
na Etapa 00 (14/10/2026), que indica Lucas como responsável por Pagamento e
Mock de Pagamento. A dupla deve decidir quem mantém esta parte.

## Fluxo e implementação

```text
Estoque --pedido.estoque_ok--> Pagamento --POST /charges--> Mock (8083)
Pagamento --pagamento.checkout_disponivel--> Gateway --SSE--> React
Cliente clica no Mock --POST /webhooks/payments (HMAC)--> Pagamento (8082)
Pagamento --pagamento.aprovado / pagamento.recusado--> Gateway (e Entrega)
```

| Arquivo | Responsabilidade |
|---|---|
| `cmd/mock-pagamento/main.go` | Inicia o Mock na porta 8083 |
| `mock-pagamento/mock.go` | `POST /charges`, decisão do cliente e envio do webhook |
| `mock-pagamento/page.go` | Página de checkout com os dois botões |
| `cmd/pagamento/main.go` | Liga banco, servidor do webhook e consumidor |
| `ms-pagamento/service.go` | Conexão RabbitMQ, fila, chaves e publicação assinada |
| `ms-pagamento/events.go` | `pedido.estoque_ok`: cria a cobrança e publica o checkout |
| `ms-pagamento/mock_client.go` | Chamada `POST /charges` com `Idempotency-Key` e novas tentativas |
| `ms-pagamento/webhook_handler.go` | `POST /webhooks/payments` e `GET /healthz` |
| `ms-pagamento/repository.go` | Cobranças e eventos de webhook no schema `pagamento` |
| `utils/webhook/signature.go` | Assinatura e validação HMAC-SHA256 |
| `migrations/011_payment_charges.sql` | Colunas novas em `pagamento.payment_charges` |

Os pacotes ficam em `microservices/`. Os eventos usam os tipos compartilhados
`events.WebPaymentCheckoutPayload` e `events.WebPaymentResultPayload`, os mesmos
validados pelo Gateway.

### Mock de Pagamento

É um sistema externo simulado: guarda as cobranças em memória e não usa o banco
do projeto.

| Requisição | Resultado |
|---|---|
| `POST /charges` nova | HTTP 201 com `charge_id`, `checkout_url` e `expires_at` |
| Mesma `Idempotency-Key` e mesmo corpo | HTTP 200 com a mesma cobrança |
| Mesma chave com outro corpo | HTTP 409 |
| Corpo inválido | HTTP 400 `INVALID_REQUEST` |
| `GET /checkout/{id}` | Página com "Pagamento Aprovado" e "Pagamento Recusado" |
| Clique em um botão | Webhook assinado; página com o resultado |
| Clique depois do prazo | HTTP 410, sem webhook |
| Clique no botão contrário | HTTP 409, sem webhook |

Cada cobrança recebe um único `event_id` de webhook. Se o Pagamento não
responder, a página pede nova tentativa e o mesmo evento é reenviado.

### MS Pagamento

Ao receber `pedido.estoque_ok`:

1. procura a cobrança do pedido;
2. se não existe, chama o Mock (`Idempotency-Key` = pedido, até 3 tentativas)
   e grava a cobrança com o `charge_id` do Mock como chave;
3. publica `pagamento.checkout_disponivel` e marca `checkout_published_at`.

Se o mesmo evento chegar de novo, nenhuma cobrança nova é criada e o checkout
só é publicado se ainda não tinha sido.

Ao receber o webhook:

| Situação | Resposta |
|---|---|
| Assinatura ausente ou inválida | HTTP 401 |
| Corpo inválido ou cobrança de outro pedido | HTTP 400 |
| Cobrança inexistente | HTTP 404 |
| Resultado diferente do já gravado | HTTP 409 |
| Processado ou repetido | HTTP 204 |
| Falha ao publicar no RabbitMQ | HTTP 500 (o Mock pode tentar de novo) |

Uma transação grava o `event_id` em `pagamento.webhook_events` e o resultado da
cobrança. Depois do commit, o resultado é publicado e `result_published_at` é
marcado. Um webhook repetido cujo resultado ainda não foi publicado é
publicado nesse momento; um já publicado não gera novo evento.

A recusa usa o motivo `Pagamento recusado pelo usuário`. O Gateway publica
`pedido.excluido` com `PAYMENT_REFUSED` e o Estoque devolve as unidades.

## Como executar

```powershell
docker compose up -d postgres
$env:PAYMENT_WEBHOOK_SECRET = "um-segredo-local-com-pelo-menos-32-bytes"
go run ./cmd/mock-pagamento
go run ./cmd/pagamento
```

Use o mesmo `PAYMENT_WEBHOOK_SECRET` nos dois terminais; ele precisa ter pelo
menos 32 bytes. `MOCK_CHECKOUT_TTL` (padrão `30m`) define a validade do
checkout. Em um banco que já existia, aplique a migration nova:

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce -f /docker-entrypoint-initdb.d/011_payment_charges.sql
```

## Validação automatizada

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./...
```

| Grupo | Testes |
|---|---|
| Assinatura HMAC | 4 |
| Mock de Pagamento | 12 |
| Cliente do Mock | 4 |
| `pedido.estoque_ok` | 7 |
| Webhook | 8 |
| Repositório no PostgreSQL | 5 |

## Validação manual

Fluxo completo pela API do Gateway, com Gateway, Estoque, Pagamento e Mock
reais, RabbitMQ e PostgreSQL:

| Cenário | Resultado |
|---|---|
| Pedido de 1 PROD-003 | estoque 12 para 11; checkout publicado; pedido `AGUARDANDO_PAGAMENTO` com `checkout_url` |
| Página do Mock | pedido, valor `R$ 119,90` e os dois botões |
| "Pagamento Aprovado" | pedido `PAGAMENTO_APROVADO`; SSE com `ESTOQUE_CONFIRMADO`, `AGUARDANDO_PAGAMENTO` e `PAGAMENTO_APROVADO` |
| Pedido de 2 PROD-013 e "Pagamento Recusado" | pedido `PAGAMENTO_RECUSADO`; Gateway publica `pedido.excluido`; estoque 8, 6 e de volta a 8 |
| Clique no botão contrário depois | HTTP 409 |
| Webhook com assinatura falsa | HTTP 401 |
| Webhook assinado repetido | HTTP 204, nenhum evento novo |

## Limitações e integração pendente

- Se o Mock estiver fora do ar nas 3 tentativas, `pedido.estoque_ok` recebe
  `Nack` sem reenfileiramento e o pedido fica em `ESTOQUE_CONFIRMADO`.
- As cobranças do Mock ficam em memória: reiniciar o Mock invalida os links
  abertos (o Pagamento mantém sua cópia no banco).
- O MS Entrega ainda é o da Avaliação 01 e precisa ser adaptado ao novo
  `pagamento.aprovado` na próxima etapa.
- O Mock já recusa cliques após `expires_at`. Lucas implementou a expiração no
  Gateway na branch `feat/checkout-expiration`, com demonstração local validada;
  consultar a [Etapa 15](../15-expiracao-checkout/README.md), inclusive os limites
  de concorrência com pagamentos atrasados. Integração dessa branch ainda pendente.

## Estado

MS Pagamento e Mock de Pagamento prontos e integrados ao Gateway e ao Estoque.
