# Etapa 16 - MS Entrega com os contratos da Avaliação 02

## Base e escopo

Branch `feat/entrega`, criada a partir de `fc6b449` (após o PR #26). Gabriel
adapta o MS Entrega da Avaliação 01 aos contratos atuais e corrige um caso de
webhook no MS Pagamento. Atende à Prioridade 1 de
`pendencias-integracao-gabriel` (seção 3) e ao teste T14.

Nota fiscal e rastreio são simulados. `pedido.enviado` representa o despacho
simulado, não a entrega física ao destinatário.

## Fluxo e implementação

```text
Pagamento --pagamento.aprovado--> Entrega (fila entrega)
Entrega: grava entrega + envelope assinado (uma por pedido)
Entrega --pedido.enviado (confirmado pelo RabbitMQ)--> Gateway (ENVIADO, SSE)
```

1. O consumidor compartilhado valida routing key, produtor (`ms-pagamento`) e
   assinatura.
2. O payload é lido como `events.WebPaymentResultPayload` e validado com as
   mesmas regras do Gateway: pedido, cliente e cobrança obrigatórios, valor
   positivo, moeda `BRL` e `processed_at`.
3. Se o pedido ainda não tem entrega, são gerados nota (`NF-AAAAMMDD-NNNNNN`),
   rastreio (`BR` + 9 dígitos + `BR`) e `shipped_at` em UTC, e o
   `pedido.enviado` (`events.WebOrderShippedPayload`) é assinado como
   `ms-entrega` **uma única vez**.
4. A entrega e o envelope assinado são gravados em `entrega.shipments` antes da
   publicação. `order_id` é a chave: uma entrega por pedido.
5. O envelope gravado é publicado com confirmação do RabbitMQ e `mandatory`
   (`rabbitmq.PublishConfirmed`). Só depois `published_at` é preenchido.
6. A mensagem recebe `Ack` somente depois da gravação. Se a publicação falhar,
   a entrega fica pendente e a rotina de pendências a reenvia a cada 10 s.

| Situação | Resultado |
|---|---|
| Primeira aprovação | entrega criada e `pedido.enviado` publicado |
| Mesma aprovação de novo (mesmo ou outro `event_id`) | nada novo; log `já enviado` |
| Aprovação repetida com publicação pendente | reenvia o **mesmo** envelope (mesmo `event_id`) |
| Aprovação de outro cliente para um pedido já entregue | rejeitada, nada publicado |
| Payload inválido | rejeitado antes de gravar |
| Falha ao gravar no banco | erro; nada publicado |
| Falha ao publicar | `Ack`, entrega pendente, reenvio automático |

O Gateway já reconhece a chave pública de `ms-entrega` e grava `invoice_id`,
`tracking_code` e `shipped_at` em `gateway.orders` ao aplicar o evento.

| Arquivo | Responsabilidade |
|---|---|
| `cmd/entrega/main.go` | Banco, RabbitMQ, consumidor e rotina de pendências |
| `ms-entrega/events.go` | Decisão, publicação e reenvio de pendências |
| `ms-entrega/shipment.go` | Nota, rastreio e validação do pagamento |
| `ms-entrega/repository.go` | `entrega.shipments` |
| `ms-entrega/service.go` | Fila, chaves, assinador e publicador confirmado |
| `utils/rabbitmq/confirmed_publisher.go` | Publicação com confirmação e `mandatory` |
| `migrations/013_delivery_shipments.sql` | Schema `entrega` e tabela de entregas |

`PublishConfirmed` é uma função nova em `utils/rabbitmq`, no mesmo padrão do
publicador confirmado do Gateway. As funções existentes não mudaram.

## Correção no MS Pagamento (T14)

Um webhook com `event_id` já usado por **outra** cobrança era tratado como
repetição e o Pagamento publicava `pagamento.aprovado` para uma cobrança ainda
`PENDING`. Agora:

- `ApplyWebhook` confere a cobrança dona do `event_id` repetido e retorna
  `ErrWebhookEventReused` (HTTP 409 `EVENT_ID_CONFLICT`), sem alterar nada;
- o webhook nunca publica resultado de cobrança sem status `APPROVED` ou
  `REFUSED`.

## Como executar

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce -f /docker-entrypoint-initdb.d/013_delivery_shipments.sql
go run ./cmd/entrega
```

Aplicar a 013 também em `ecommerce_events_test`. Iniciar a Entrega junto com
Gateway, Estoque, Pagamento e Mock (Etapas 13 e 14).

## Validação automatizada

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./...
$env:TEST_RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
go test ./utils/rabbitmq/
```

| Grupo | Testes |
|---|---|
| Entrega: regras, repetição, pendências | 10 |
| Entrega: repositório no PostgreSQL | 3 |
| Publicação confirmada (RabbitMQ real, exchange e fila temporárias) | 3 |
| Pagamento: `event_id` reutilizado e cobrança sem resultado | 3 |

Suíte completa: 150 PASS, 0 FAIL. Os testes de RabbitMQ criam exchange e fila
com nome aleatório e as apagam no fim; não usam as filas do projeto.

## Validação manual

Broker: RabbitMQ nativo do Windows, `127.0.0.1:5672`, vhost `/`, painel
`http://127.0.0.1:15672`. PostgreSQL do Docker. Pedidos criados pela API do
Gateway com Gateway, Estoque, Pagamento, Mock e Entrega rodando.

| Cenário | Resultado |
|---|---|
| T01: PED-004 aprovado | `ENVIADO`; `gateway.orders` com nota `NF-20261015-823796`, rastreio `BR735569943BR` e `shipped_at`; SSE recebeu `ESTOQUE_CONFIRMADO`, `AGUARDANDO_PAGAMENTO`, `PAGAMENTO_APROVADO`, `ENVIADO` |
| T02: PED-005 recusado | `PAGAMENTO_RECUSADO`; nenhuma entrega; reserva `RELEASED` |
| Entrega parada durante a aprovação de PED-006 | aprovação aguardou na fila `entrega`; ao reiniciar, `ENVIADO` com nota e rastreio |
| T06 (lado da Entrega): `pagamento.aprovado` de PED-004 repetido com novo `event_id` | uma única entrega, nenhum `pedido.enviado` novo, log `aprovação repetida ignorada` |

No T06, o Gateway também recebeu a aprovação repetida para um pedido já
`ENVIADO` e passou a reprocessá-la (`RECEBIDO`, tentativa 2 de 12); ao fim das
tentativas ela vai para `PENDENTE_REVISAO`. O pedido continua `ENVIADO`. Esse
reconhecimento de resultado equivalente é o item 4.3 das pendências, do lado do
Gateway.

## Limitações e integração pendente

- Falha ao **gravar** no banco ainda faz o consumidor compartilhado descartar a
  mensagem (`Nack` sem reenfileirar). A correção (tentativas com intervalo e
  pendência persistida) é o próximo passo e altera `utils/rabbitmq`.
- Entrega não sabe se o pedido foi cancelado por expiração. A regra entre
  pagamento atrasado e expiração (seção 6 das pendências) ainda será
  combinada com Lucas.
- `GET /api/orders/{orderId}/tracking` não existe no Gateway; nota e rastreio
  estão gravados em `gateway.orders`, mas a exibição ao cliente depende de Lucas.
- A fila antiga `principal` recebe cópias de `pedido.enviado` sem consumidor.

## Estado

MS Entrega adaptado e integrado: o fluxo do PDF chega a "Pedido Enviado".

## Atualização (Etapa 17)

A Entrega passou a usar o consumidor confiável: falha ao gravar no banco tem
novas tentativas e depois vai para `entrega.revisao`
(`go run ./cmd/reprocessar -fila entrega`), em vez de descartar a mensagem.
Payload inválido e cliente divergente são rejeitados sem novas tentativas.
Nota, rastreio e data aparecem no detalhe do pedido e na tela. Detalhes na
[Etapa 17](../17-confiabilidade-expiracao/README.md).
