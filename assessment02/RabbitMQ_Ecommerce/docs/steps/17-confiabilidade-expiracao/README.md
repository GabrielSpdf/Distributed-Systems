# Etapa 17 - Confiabilidade, autoridade do prazo e ajustes finais

## Base e escopo

Branch `feat/integracao-final`, sobre `fc6b449` (PR #26) e a Etapa 16. Fecha as
pendências de `pendencias-integracao-gabriel` (seções 4 a 9) e as 11 perguntas
enviadas ao Lucas, que delegou as decisões. A validação ponta a ponta está na
[Etapa 18](../18-validacao-integracao/README.md) e o roteiro de apresentação em
[docs/demo](../../demo/README.md).

## Decisões

| # | Ponto | Decisão |
|---|---|---|
| 1 | Consumidor compartilhado | Novo `rabbitmq.ConsumeSignedEventsReliably`: mensagem inválida é rejeitada; falha temporária tem novas tentativas e depois vai para `<fila>.revisao`. O `ConsumeSignedEvents` antigo continua para os comandos da Avaliação 01. |
| 2 | Publicação confirmada | Estoque e Pagamento gravam o envelope assinado antes de publicar, publicam com confirmação e reenviam pendências, como a Entrega. |
| 3 | Pagamento × expiração | O MS Pagamento é a autoridade do prazo. Expira cobranças vencidas e publica `pagamento.recusado` com `CHECKOUT_EXPIRED`; o Gateway cancela o pedido. O executor do Gateway virou rede de segurança (10 min). |
| 4 | Resultado equivalente no inbox | Sem mudança no Gateway: nossos serviços passam a reenviar sempre o mesmo `event_id`, que o inbox já descarta. |
| 5 | Rastreio | Nota, rastreio e data entram no detalhe/listagem do pedido e na tela; a rota `/tracking` saiu do contrato. |
| 6 | Método QUERY | Retirado do contrato (não exigido pelo enunciado). |
| 7 | Filas antigas | `principal`, `consumidor.01` e `consumidor.02` (vazias) removidas do broker da demonstração. |
| 8 | Broker | RabbitMQ nativo do Windows, `amqp://guest:guest@127.0.0.1:5672/`, vhost `/`, painel `http://127.0.0.1:15672`; Docker só com PostgreSQL. |
| 9 | Conhecimento | Os dois explicam todo o código; sessão de apresentação mútua. |
| 10 | Matriz T01–T20 | Executada pelo Gabriel (Etapa 18). |
| 11 | E-mail na inscrição | `POST /api/promotion-interests` aceita `email`; a tela tem o campo. |

## Consumidor confiável (`utils/rabbitmq/reliable_consumer.go`)

```text
mensagem → JSON? routing key? produtor? assinatura? ──não──► Nack (descarta) + log [SEGURANÇA]
             │ sim
             ▼
          handler ──nil──► Ack
             │
             ├─ erro rabbitmq.Permanent ──► Nack + log
             │
             └─ outro erro ──► nova tentativa (1, 2, 4, 8 s; até 5)
                                 └─ esgotou ──► publica a mensagem original em
                                                <fila>.revisao (confirmada) ──► Ack
                                                (se nem isso der certo: Nack com requeue)
```

- Prefetch 1: uma mensagem por vez, em ordem.
- Cabeçalhos da revisão: `x-revisao-motivo`, `x-revisao-tentativas`,
  `x-revisao-em`, `x-fila-origem`, `x-exchange-origem`, `x-routing-key-origem`.
- `go run ./cmd/reprocessar -fila <estoque|pagamento|entrega>` devolve as
  mensagens à fila original. Como chegam pela exchange padrão, o consumidor usa
  `x-routing-key-origem` na conferência da routing key; assinatura e produtor
  continuam verificados.
- Handlers marcam como permanentes: payload inválido, evento inesperado,
  `ErrInvalidOrder`, cliente divergente. Banco, Mock e RabbitMQ são temporários.

## Publicação confirmada (`utils/rabbitmq/confirmed_publisher.go`)

`PublishConfirmed` abre um canal próprio, ativa publisher confirms, publica com
`mandatory` e só retorna sucesso com o ACK do broker e sem `basic.return`
(mensagem sem fila de destino é erro).

| Serviço | Envelope gravado em | Rotina de reenvio |
|---|---|---|
| Estoque | `estoque.stock_reservations.result_envelope` (+ `shortfalls` originais) | a cada 10 s, resultados `RESERVED`/`UNAVAILABLE` não publicados |
| Pagamento | `pagamento.payment_charges.checkout_envelope` e `result_envelope` | a cada 10 s, checkouts e depois resultados |
| Entrega | `entrega.shipments.envelope` | a cada 10 s |

O primeiro envelope gravado vence (`WHERE ... IS NULL`), então todo reenvio usa o
mesmo `event_id`. Um `estoque.indisponivel` repetido usa as faltas originais,
mesmo depois de o estoque ser reposto. Com o envelope gravado, falha do RabbitMQ
não rejeita a mensagem de entrada: a rotina publica depois. No webhook do
Pagamento, isso significa `204` (o resultado está gravado); `500` só se o
envelope não puder ser gravado.

O `SignedPublisher` com mutex do Pagamento foi substituído por
`ConfirmedEventBus` (canal próprio por publicação).

## Autoridade do prazo (MS Pagamento)

- `ExpireNextCharge` trava uma cobrança `PENDING` vencida há mais de 5 s
  (`FOR UPDATE SKIP LOCKED`), marca `EXPIRED`, `reason = CHECKOUT_EXPIRED`.
- `RunChargeExpiration` roda a cada 5 s e publica `pagamento.recusado` com
  `reason: "CHECKOUT_EXPIRED"` pelo mesmo caminho de envelope gravado.
- `ApplyWebhook`: cobrança expirada → `409 CONFLICTING_RESULT`; `occurred_at`
  depois de `expires_at` → `409 CHECKOUT_EXPIRED`.
- Mock: resposta `409` do Pagamento vira recusa definitiva ("A loja não aceitou
  o pagamento: o checkout expirou"), sem novas tentativas.

Ajustes no Gateway (código do Lucas, mínimos):

- `pagamento.recusado` com `CHECKOUT_EXPIRED` → `CANCELADO` e compensação
  `pedido.excluido` com `CHECKOUT_EXPIRED` (`status_repository.go`), e o mesmo
  status no SSE (`order_sse.go`), via `isCheckoutExpiredRefusal`;
- margem do executor de expiração de 30 s para 10 min
  (`checkout_expiration.go`), agora só rede de segurança.

## Rastreio no pedido

`Order` ganhou `invoice_id`, `tracking_code` e `shipped_at` (omitidos enquanto
vazios) na listagem e no detalhe; a tela de pedidos mostra
"Enviado em … · Nota fiscal … · Rastreio …".

## E-mail na inscrição de promoções

- Migration `015_promotion_preference_email.sql`: coluna `email` em
  `gateway.promotion_preferences`.
- `SubscribeWithEmail` normaliza e valida o e-mail (`net/mail`, sem nome de
  exibição, até 254 caracteres); vazio mantém o atual ou usa o da conta.
- Trocar só o e-mail gera `interesse.promocao` com todas as categorias ativas.
- `GET /api/promotion-interests` devolve o e-mail; a tela tem o campo e o botão
  "Atualizar e-mail".
- `Subscribe` (assinatura antiga) continua existindo e delega.

## Arquivos

| Arquivo | Mudança |
|---|---|
| `utils/rabbitmq/reliable_consumer.go`, `review.go`, `confirmed_publisher.go` | consumidor confiável, revisão, publicação confirmada |
| `cmd/reprocessar/main.go` | reprocessamento da revisão |
| `migrations/014_reliable_results.sql` | colunas de envelope e faltas |
| `migrations/015_promotion_preference_email.sql` | e-mail da preferência |
| `ms-estoque/{repository,events,service,model}.go`, `cmd/estoque` | envelope gravado, consumidor confiável |
| `ms-pagamento/{outbox,expiration,events,webhook_handler,repository,service,model}.go`, `cmd/pagamento` | outbox, autoridade do prazo |
| `mock-pagamento/{mock,page}.go` | recusa definitiva no 409 |
| `ms-entrega/events.go`, `cmd/entrega` | erros permanentes, consumidor confiável |
| `ms-gateway/orders/{status_repository,order_sse,status_transition,checkout_expiration,model,repository}.go` | expiração pelo Pagamento, rastreio |
| `ms-gateway/promotions/{model,repository,http_handler}.go` | e-mail na inscrição |
| `frontend/src/{api.ts,Orders.tsx,PromotionPreferences.tsx,styles.css}` | rastreio e campo de e-mail |

## Testes novos

Consumidor confiável e publicação confirmada (RabbitMQ real, recursos
temporários), repositórios com PostgreSQL real (faltas originais, envelopes,
pendências, expiração, webhook atrasado), handlers com stubs (erros permanentes
e temporários, reenvio do envelope gravado), Gateway (cancelamento por
expiração, SSE, rastreio no detalhe, margem de 10 min) e promoções (e-mail).

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./...
$env:TEST_RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
go test ./utils/rabbitmq/
cd frontend; npm run build
```

Resultado: 182 PASS, 0 FAIL; os 11 SKIP são testes de RabbitMQ que exigem
`TEST_RABBITMQ_URL` (os 9 de `utils/rabbitmq` passam na execução com a
variável; os 2 do Gateway publicam nas filas reais e não foram executados).

## Limitações

- Os serviços não reconectam sozinhos ao RabbitMQ: se o broker derrubar a
  conexão, o processo encerra com log claro e deve ser reiniciado (mensagens
  ficam nas filas duráveis; pendências são reenviadas).
- Com o Estoque fora, o Gateway não cria pedidos (o catálogo é conferido por
  REST).
- Único caso de corrida que resta: se o Pagamento cair logo depois de gravar
  uma aprovação e ficar fora por mais de 10 minutos após o prazo, o executor de
  segurança do Gateway cancela o pedido e o estoque é devolvido. Quando o
  Pagamento volta, publica a aprovação: o Gateway a manda para revisão, mas a
  Entrega gera o envio. Exige queda longa do Pagamento exatamente nessa janela;
  não foi tratado.
- O Pagamento não consome `pedido.excluido`: se o cliente cancelar antes de
  pagar, a cobrança fica pendente e, ao vencer, gera um `pagamento.recusado`
  (`CHECKOUT_EXPIRED`) que o Gateway não consegue aplicar a um pedido já
  cancelado; a linha vai para revisão no inbox, sem efeito no pedido ou no
  estoque.
- Cobranças do Mock continuam em memória.
- Resend restrito ao destinatário de teste.
