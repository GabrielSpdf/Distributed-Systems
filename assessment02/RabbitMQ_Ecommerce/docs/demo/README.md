# Roteiro de execução e demonstração

Todos os comandos partem de `assessment02/RabbitMQ_Ecommerce`, um processo por
terminal PowerShell. Variáveis definidas em um terminal **não** aparecem nos
outros nem sobrevivem ao reinício do computador; defina-as em cada terminal
que as usa.

## 1. Infraestrutura

| Item | Valor |
|---|---|
| RabbitMQ | serviço nativo do Windows, `amqp://guest:guest@127.0.0.1:5672/`, vhost `/` |
| Painel do RabbitMQ | `http://127.0.0.1:15672` (guest/guest) |
| PostgreSQL | Docker: `docker compose up -d postgres` (não subir o `rabbitmq` do Compose) |
| Portas | Gateway 8080, Estoque 8081, Pagamento 8082, Mock 8083, frontend 5173 |

Conferir que só existe um broker escutando:

```powershell
Get-Service '*Rabbit*'
Get-NetTCPConnection -State Listen | Where-Object { $_.LocalPort -in @(5672, 15672) } |
    Select-Object LocalAddress, LocalPort, OwningProcess
```

Não usar `docker compose exec rabbitmq ...` como prova do estado do broker
nativo. Não rodar `cmd/mock-stock` junto com o Estoque real (mesma porta).

### Banco que já existia

Aplicar uma vez cada migration ainda não aplicada (as do Lucas, 007–009, e as
novas). O caminho dentro do container preserva os acentos:

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce -v ON_ERROR_STOP=1 -f /docker-entrypoint-initdb.d/013_delivery_shipments.sql
docker compose exec -T postgres psql -U ecommerce -d ecommerce -v ON_ERROR_STOP=1 -f /docker-entrypoint-initdb.d/014_reliable_results.sql
docker compose exec -T postgres psql -U ecommerce -d ecommerce -v ON_ERROR_STOP=1 -f /docker-entrypoint-initdb.d/015_promotion_preference_email.sql
```

Um volume novo executa 001–015 sozinho. Chaves: `go run ./cmd/keygen` só em
máquina nova; para Promoções, `go run ./cmd/keygen -promotions-only`. Nunca
regenerar todas as chaves.

## 2. Variáveis comuns

Em cada terminal de serviço:

```powershell
$env:RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
$env:DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce?sslmode=disable"
```

Também:

- Pagamento e Mock: o **mesmo** `$env:PAYMENT_WEBHOOK_SECRET` (32+ bytes);
- Gateway: `$env:JWT_SECRET` (32+ bytes, sempre o mesmo);
- Promoções: `$env:RESEND_FROM`, `$env:RESEND_TEST_RECIPIENT` e, só no
  executor de e-mails, `$env:RESEND_API_KEY` (ver Etapa 12).

## 3. Ordem de início

Consumidores antes de criar pedidos ou ofertas:

```powershell
go run ./cmd/estoque                 # 8081
go run ./cmd/mock-pagamento          # 8083 (opcional: $env:MOCK_CHECKOUT_TTL = "2m")
go run ./cmd/pagamento               # 8082
go run ./cmd/entrega
go run ./cmd/promocoes -interests
go run ./cmd/promocoes -offers
go run ./cmd/promocoes -emails       # só com RESEND_API_KEY
go run ./cmd/gateway                 # 8080
cd frontend; npm run dev             # 5173
```

Iniciar o Mock antes dos pedidos e não reiniciá-lo durante a demonstração
(cobranças em memória). Antes de iniciar o Gateway, conferir pedidos antigos em
`AGUARDANDO_PAGAMENTO`: depois de 10 min vencidos, a rede de segurança cancela.

Conferência rápida:

```powershell
foreach ($p in 8080, 8081, 8082, 8083) { (Invoke-WebRequest "http://localhost:$p/healthz").Content }
```

No painel, cada fila do fluxo (`estoque`, `pagamento`, `entrega`,
`gateway.pedidos.status`, `promocoes.interesses`, `promocoes.ofertas`) deve
ter 1 consumidor.

## 4. Demonstração sugerida

1. **Catálogo** vindo do Estoque: `http://localhost:8081/internal/products` e a
   tela do React.
2. **Pedido aprovado**: comprar, ver `ESTOQUE_CONFIRMADO` → `AGUARDANDO_PAGAMENTO`
   sem recarregar, "Ir para pagamento" (nova aba do Mock), "Pagamento Aprovado",
   ver `PAGAMENTO_APROVADO` → `ENVIADO` com nota e rastreio na tela.
3. **Recusa**: outro pedido, "Pagamento Recusado", estoque devolvido.
4. **Checkout expirado** (Mock com `MOCK_CHECKOUT_TTL = "2m"`): não pagar; ~5 s
   depois do prazo o Pagamento expira e o pedido fica `CANCELADO`.
5. **Sem estoque**: pedir PROD-014 → `ESTOQUE_INDISPONIVEL`.
6. **Promoções**: informar o e-mail e uma categoria, publicar uma oferta
   (`go run ./cmd/publish-promotions -once -category alimentos`).
7. **Segurança**: no painel, mostrar um envelope com `signature`; explicar o HMAC
   do webhook.

Consultas úteis:

```sql
SELECT order_id, status, shortfalls IS NOT NULL AS faltas, result_published_at FROM estoque.stock_reservations ORDER BY created_at DESC LIMIT 5;
SELECT order_id, status, reason, checkout_published_at, result_published_at FROM pagamento.payment_charges ORDER BY created_at DESC LIMIT 5;
SELECT order_id, invoice_id, tracking_code, published_at FROM entrega.shipments ORDER BY created_at DESC LIMIT 5;
SELECT display_id, status, tracking_code FROM gateway.orders ORDER BY created_at DESC LIMIT 5;
```

## 5. Recuperar pendências

| Sintoma | O que fazer |
|---|---|
| Mensagem em `estoque.revisao`, `pagamento.revisao` ou `entrega.revisao` | Ver o motivo no cabeçalho `x-revisao-motivo` (painel → fila → Get messages). Resolver a causa (banco, Mock) e rodar `go run ./cmd/reprocessar -fila <fila>` |
| Serviço encerrou com "canal RabbitMQ fechado" | O broker caiu ou derrubou a conexão: reiniciar o serviço. Mensagens esperam nas filas duráveis; eventos gravados e não publicados são reenviados em até 10 s |
| Evento publicado não chegou | Conferir no painel se a fila de destino tem consumidor; eventos de Estoque/Pagamento/Entrega só são marcados como publicados com confirmação do broker |
| Linha do inbox do Gateway em `PENDENTE_REVISAO` | Evento que não pôde ser aplicado ao estado do pedido; analisar antes de qualquer ação |

Não apagar filas, volumes nem mensagens para "resolver" uma divergência.

## 6. Testes

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./...
$env:TEST_RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
go test ./utils/rabbitmq/
cd frontend; npm run build
```

`TEST_RABBITMQ_URL` só com `./utils/rabbitmq/`: esses testes usam exchange e
filas temporárias; os testes de RabbitMQ do Gateway publicam nas exchanges reais.
