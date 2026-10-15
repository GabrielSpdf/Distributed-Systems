# Etapa 12 - MS Estoque com PostgreSQL

## Base e escopo

Branch `feat/estoque-postgres`, criada a partir de `c30bed3`, após o PR #22.
Gabriel substitui o Estoque da Avaliação 01, baseado em arquivos JSON, por um
serviço que usa o schema `estoque` do PostgreSQL, expõe o catálogo por REST e
segue os contratos da Etapa 02. Pagamento, Entrega e Promoções ficam para as
próximas etapas.

## Fluxo e implementação

```text
Gateway --GET /internal/products--> MS Estoque --> estoque.products

Gateway --pedido.criado--> MS Estoque --pedido.estoque_ok / estoque.indisponivel--> Gateway
Gateway --pedido.excluido--> MS Estoque (devolve as unidades)
```

Um único processo (`go run ./cmd/estoque`) mantém o servidor HTTP na porta
8081 (`STOCK_PORT`) e o consumidor RabbitMQ da fila `estoque`.

| Arquivo | Responsabilidade |
|---|---|
| `cmd/estoque/main.go` | Liga banco, servidor HTTP e consumidor |
| `ms-estoque/service.go` | Conexão RabbitMQ, fila, bindings, chaves e publicação assinada |
| `ms-estoque/http_handler.go` | `GET /internal/products` e `GET /healthz` |
| `ms-estoque/events.go` | Leitura dos eventos e decisão do que publicar |
| `ms-estoque/repository.go` | Consultas e transações no PostgreSQL |
| `ms-estoque/reservation.go` | Regras puras: somar itens repetidos e achar faltas |
| `ms-estoque/model.go` | Tipos e situações da reserva |
| `migrations/007_stock_catalog.sql` | Produtos PROD-004 a PROD-014 |

Os arquivos `ms-estoque/...` ficam em `microservices/ms-estoque`. Os payloads
usam os tipos compartilhados `events.WebOrderCreatedPayload`,
`events.WebOrderDeletedPayload`, `events.WebStockConfirmedPayload` e
`events.WebStockUnavailablePayload`, os mesmos lidos pelo Gateway.

### Endpoint interno

```http
GET /internal/products?category=alimentos&available=true
```

- `category`: opcional, comparada em minúsculas;
- `available`: opcional, `true` (quantidade maior que zero) ou `false` (zerado);
- somente produtos ativos, ordenados pelo identificador.

| Situação | Resposta |
|---|---|
| Consulta atendida | HTTP 200 com `products` e `total` |
| `available` diferente de true ou false | HTTP 400, código `INVALID_REQUEST` |
| Falha no banco | HTTP 500, código `INTERNAL_ERROR` |

`GET /healthz` responde 200 quando o PostgreSQL está acessível e 503 quando não.

### Reserva

Ao receber `pedido.criado`, o repositório executa uma única transação:

1. procura uma reserva já registrada para o `order_id`;
2. bloqueia as linhas dos produtos pedidos (`SELECT ... FOR UPDATE`);
3. compara o pedido com o estoque;
4. se faltar qualquer item, grava `UNAVAILABLE` e não altera nenhuma quantidade;
5. se tudo estiver disponível, baixa as quantidades e grava a reserva
   `RESERVED` com seus itens.

Não existe reserva parcial. O bloqueio das linhas impede que dois pedidos
simultâneos levem as mesmas unidades.

| Situação gravada | Evento publicado |
|---|---|
| `RESERVED` | `pedido.estoque_ok` |
| `UNAVAILABLE` | `estoque.indisponivel` com `unavailable_items` |
| `RELEASED` | nenhum |

### Idempotência

A chave primária de `estoque.stock_reservations` é o `order_id`. Se o mesmo
`pedido.criado` chegar de novo, o estoque não é baixado outra vez: o serviço
republica o resultado gravado, com o mesmo `reserved_at`. Se um pedido
`UNAVAILABLE` chegar de novo depois de o estoque ser reposto, nada é
publicado, porque o Gateway rejeita `estoque.indisponivel` sem itens.

### Cancelamento

Ao receber `pedido.excluido`:

- reserva `RESERVED`: devolve as quantidades e marca `RELEASED`;
- reserva `UNAVAILABLE` ou `RELEASED`: nada a devolver;
- nenhuma reserva ainda: grava `RELEASED` (cancelamento preventivo), o que
  impede uma reserva caso o `pedido.criado` chegue depois.

### Confirmação das mensagens

O consumidor compartilhado valida routing key, produtor e assinatura antes de
chamar o serviço. O `Ack` só acontece depois da gravação no banco e da
publicação do resultado. Em caso de erro, a mensagem recebe `Nack` sem
reenfileiramento.

## Como executar

```powershell
docker compose up -d postgres
go run ./cmd/estoque
```

O RabbitMQ precisa estar acessível em `localhost:5672`. Em um banco que já
existia, aplique a migration nova uma vez:

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce -f /docker-entrypoint-initdb.d/007_stock_catalog.sql
```

Não execute `cmd/mock-stock` ao mesmo tempo: os dois usam a porta 8081.

## Validação automatizada

```powershell
go test ./...
```

Os testes do repositório usam o PostgreSQL real e só rodam com a variável.
Use o mesmo banco de testes da Etapa 08 (`ecommerce_events_test`, com as
migrations 001 a 007), pois os testes do Gateway recusam outro banco:

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./...
```

Os testes do Estoque criam produtos próprios com prefixo `TEST-` e os removem
ao final.

| Grupo | Testes |
|---|---|
| Regras de reserva | 4 |
| Repositório no PostgreSQL | 6 |
| Endpoint HTTP | 6 |
| Eventos | 12 |

## Validação manual

Validada com RabbitMQ e PostgreSQL reais, publicando eventos assinados como
`ms-principal` e capturando as respostas em uma fila temporária.

| Cenário | Resultado |
|---|---|
| Catálogo completo | 14 produtos |
| `category=limpeza&available=true` | PROD-002, PROD-009, PROD-010 e PROD-011 |
| `available=talvez` | HTTP 400 `INVALID_REQUEST` |
| Pedido com estoque | `pedido.estoque_ok` assinado; PROD-003 de 12 para 10 |
| Mesmo pedido recebido duas vezes | mesmo `reserved_at` republicado, estoque inalterado |
| Pedido sem estoque | `estoque.indisponivel` com PROD-014 pedido 1, disponível 0 |
| Cancelamento repetido | quantidade devolvida uma única vez |
| Cancelamento antes do pedido | pedido posterior não reserva e nada é publicado |
| Gateway consultando o Estoque real | `/api/products` com 14 itens; eletrônicos disponíveis: 3 |

## Limitações e integração pendente

- Se a publicação falhar com o canal ainda aberto, a mensagem recebe `Nack` e
  a reserva permanece gravada sem que o Gateway seja avisado. Uma solução
  completa exigiria outbox também no Estoque.
- O `docker-compose.yml` passou a montar a pasta `migrations` inteira, então
  um volume novo executa as migrations 001 a 007 em ordem (validado em um
  container descartável). Lucas, confirme se isso combina com o seu fluxo de
  aplicar as migrations manualmente.
- O fluxo completo pela interface (criar pedido no React e ver o status mudar)
  ainda precisa ser executado com o Gateway e o Estoque juntos.

## Estado

MS Estoque real pronto para substituir `cmd/mock-stock`.
`STOCK_SERVICE_URL=http://localhost:8081` continua valendo.
