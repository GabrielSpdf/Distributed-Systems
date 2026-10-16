# Etapa 18 - Validação de integração (matriz T01–T20)

## Ambiente

| Item | Valor |
|---|---|
| Código | `fc6b449` + Etapas 16 e 17 (branch `feat/integracao-final`) |
| Data | 15/10/2026, 22:10–22:30 |
| Broker | RabbitMQ 4.3.5 nativo do Windows, `127.0.0.1:5672`, vhost `/`, painel `http://127.0.0.1:15672` |
| Banco | PostgreSQL 17 no Docker, banco `ecommerce`, migrations 001–015 |
| Processos | Gateway, Estoque, Pagamento, Mock, Entrega, `promocoes -interests`, `promocoes -offers` |
| Clientes | A (`gabriel.e2e.*@example.com`) e B (criado no T17), pela API do Gateway |

Mensagens forjadas (T05, T06, T12, T15) foram publicadas por uma ferramenta
temporária que assina com as chaves locais; ela não faz parte do repositório.
Falhas foram provocadas só em processos próprios e no PostgreSQL local.

## Topologia conferida no broker

| Exchange | Routing key | Fila |
|---|---|---|
| `eCommerce` (direct, durável) | `pedido.criado`, `pedido.excluido` | `estoque` |
| | `pedido.estoque_ok` | `pagamento`, `gateway.pedidos.status` |
| | `estoque.indisponivel`, `pagamento.checkout_disponivel`, `pagamento.recusado`, `pedido.enviado` | `gateway.pedidos.status` |
| | `pagamento.aprovado` | `entrega`, `gateway.pedidos.status` |
| | `interesse.promocao` | `promocoes.interesses` |
| `Promocoes` (topic, durável) | `promocao.categoria.*` | `promocoes.ofertas` |

Todas as filas são duráveis e cada fila do fluxo web tinha 1 consumidor ativo
(7 conexões). Filas de revisão `estoque.revisao`, `pagamento.revisao` e
`entrega.revisao` criadas pelos serviços. As filas antigas `principal`,
`consumidor.01` e `consumidor.02` estavam vazias, sem consumidor, e foram
removidas (`DELETE /api/queues/%2f/<fila>?if-empty=true&if-unused=true`).

## Resultado

| ID | Cenário | Resultado | Evidência |
|---|---|---|---|
| T01 | Estoque e pagamento aprovado | ✅ | PED-007: PROD-008 19→18; cobrança `APPROVED` com checkout e resultado publicados; 1 entrega; Gateway `ENVIADO` com nota `NF-20261016-450803` e rastreio `BR374127308BR` no detalhe; SSE: `ESTOQUE_CONFIRMADO`, `AGUARDANDO_PAGAMENTO`, `PAGAMENTO_APROVADO`, `ENVIADO` |
| T02 | Pagamento recusado | ✅ | PED-008: PROD-010 22→20→22; cobrança `REFUSED`; 0 entregas; compensação `PAYMENT_REFUSED` publicada; reserva `RELEASED` |
| T03 | Checkout vencido sem pagamento | ✅ | PED-010 (checkout de 1 min): `CANCELADO` ~70 s depois; cobrança `EXPIRED`/`CHECKOUT_EXPIRED`; compensação `CHECKOUT_EXPIRED` publicada; PROD-002 50→49→50; link de pagamento ausente na API; clique no Mock depois: 410. PED-011: SSE com `CANCELADO` |
| T04 | Um item sem estoque | ✅ | PED-009 (PROD-013 + PROD-014): `ESTOQUE_INDISPONIVEL`; nenhuma quantidade alterada; faltas gravadas; nenhuma cobrança |
| T05 | Mesmo `event_id` repetido | ✅ | PED-007: `pedido.criado` 2× ao Estoque e os envelopes gravados reenviados ao Pagamento, à Entrega e ao Gateway → estoque, cobrança, entrega e inbox (4 linhas) inalterados; logs "repetição ignorada" |
| T06 | Resultado equivalente com novo ID | ✅ (nossos serviços) | Pagamento e Entrega ignoraram `pedido.estoque_ok` e `pagamento.aprovado` com `event_id` novo. O Gateway os guardou como `RECEBIDO` e manda para revisão: caso residual, só reproduzido por injeção manual (decisão 4 da Etapa 17) |
| T07 | Reinício antes do Ack | ✅ | Estoque encerrado à força durante as tentativas: mensagem passou de `sem_ack=1` para `prontas=1`; reiniciado, processou uma vez, sem alterar o estoque |
| T08 | Falha entre gravar e publicar | ✅ | Marcas de publicação de PED-007 apagadas no Estoque, Pagamento e Entrega: reenviados em ~5 s com os mesmos `event_id`; inbox do Gateway continuou com 6 linhas |
| T09 | Mock indisponível | ✅ | PED-012 com o Mock parado: 5 tentativas, mensagem em `pagamento.revisao` (~32 s) com motivo e routing key original; Mock de volta e `cmd/reprocessar -fila pagamento` → `AGUARDANDO_PAGAMENTO`; depois aprovado → `ENVIADO` |
| T10 | PostgreSQL indisponível | ✅ | Queda de ~6 s: 2 tentativas falharam, a seguinte processou. Queda longa: mensagem em `estoque.revisao` em ~21 s; banco de volta e `cmd/reprocessar -fila estoque` → processada; estoque inalterado |
| T11 | RabbitMQ indisponível | ✅ com ressalva | Conexões derrubadas pelo broker (API de gerenciamento): os serviços encerram com "canal RabbitMQ fechado"; com Pagamento e Entrega fora, PED-014 esperou na fila `pagamento` e chegou a `ENVIADO` depois do reinício. Reconexão automática não existe (reinício manual documentado). Com o Estoque fora, o Gateway recusa novos pedidos |
| T12 | Assinatura adulterada, produtor errado, rota incompatível | ✅ | 3 mensagens rejeitadas com motivo registrado (`verification error`, produtor `ms-estoque` não autorizado, routing key diferente do tipo); nenhuma reserva nova; nada na revisão; sem laço |
| T13 | Webhook sem HMAC válido | ✅ | 401; cobrança `PENDING`, pedido `AGUARDANDO_PAGAMENTO` |
| T14 | Webhook repetido, conflitante, ID reutilizado | ✅ | Repetido: 204 sem nova publicação; oposto: 409; `event_id` de outra cobrança: 409 `EVENT_ID_CONFLICT`, cobrança alvo continua `PENDING` |
| T15 | Cancelamento repetido/concorrente | ✅ | 3 `pedido.excluido` simultâneos: PROD-001 29→30 uma vez; o 4º (recusa) não devolveu de novo; `pedido.criado` tardio ignorado (`RELEASED`) |
| T16 | Aprovação atrasada | ✅ / ⚠️ | Aprovação assinada para cobrança expirada: 409, nenhuma entrega; decisão com `occurred_at` após o prazo: 409 `CHECKOUT_EXPIRED`. **Não reproduzido ao vivo:** `pedido.enviado` chegando ao Gateway antes da aprovação; coberto pelos testes de reprocessamento do inbox (Etapa 08) |
| T17 | Dois clientes | ✅ | B recebe 404 ao ler e ao cancelar o pedido de A; lista de B vazia; SSE de B não recebeu nenhum dos 4 eventos de A |
| T18 | Reconexão do SSE e catálogo | ✅ (API) | O Gateway encerra o SSE a cada 1 min; cliente reconectou 12 vezes sem perder eventos; quantidade do catálogo na API igual à do banco. Atualização automática da tela vem do código do Lucas (Etapa 12/PR #25), não testada por tela aqui |
| T19 | Interesse ativo e inativo | ✅ (até o job) | Inscrição com e-mail informado (202, normalizado), e-mail inválido 400; interesse no MS Promoções; oferta gerou 1 job; após cancelar, oferta não gerou job. Envio real pelo Resend não executado (sem chave nesta máquina) |
| T20 | Mesma oferta duas vezes | ✅ (até o job) | Oferta publicada 2× com o mesmo `event_id`: 1 job por evento e usuário |

## Evidências brutas

Registro gerado durante a execução (horário de Brasília):

```text
T01 PED-007: PROD-008 19->18; reserva=RESERVED; cobrança=APPROVED checkout_pub=true result_pub=true; entregas=1; gateway=ENVIADO NF-20261016-450803 BR374127308BR
T01 PED-007: SSE -> ESTOQUE_CONFIRMADO AGUARDANDO_PAGAMENTO PAGAMENTO_APROVADO ENVIADO
T02 PED-008: PROD-010 22->20->22; reserva=RELEASED; cobrança=REFUSED; entregas=0; compensação=PUBLICADO PAYMENT_REFUSED
T03 PED-010: AGUARDANDO_PAGAMENTO -> CANCELADO em ~70 s; PROD-002 50->49->50; cobrança=EXPIRED CHECKOUT_EXPIRED; compensação=PUBLICADO CHECKOUT_EXPIRED
T03 PED-011: eventos SSE do pedido: ESTOQUE_CONFIRMADO AGUARDANDO_PAGAMENTO CANCELADO
T04 PED-009: ESTOQUE_INDISPONIVEL; faltas=[{"product_id":"PROD-014","requested_quantity":1,"available_quantity":0}]; cobranças=0
T05 PED-007: antes/depois idênticos: PROD-008=18; cobranças=1; entregas=1; inbox=4 (PROCESSADO)
T07 PED-012: sem_ack=1 -> (processo encerrado) prontas=1 -> (reiniciado) prontas=0 sem_ack=0
T08 PED-007: republicados (estoque, pagamento, entrega)=truetruetrue; inbox do Gateway=6 (mesmos event_id)
T09 PED-012: pagamento.revisao=1 após ~32 s, tentativas=5; reprocessar -> AGUARDANDO_PAGAMENTO
T10b PED-012: estoque.revisao=1 após ~21 s; reprocessar -> processada; PROD-005 16->16
T12: [SEGURANÇA] ... assinatura inválida: crypto/rsa: verification error
T12: [SEGURANÇA] ... produtor ms-estoque não está autorizado a publicar o evento pedido.criado
T12: [SEGURANÇA] ... routing key pedido.criado diferente do tipo pedido.excluido
T14 PED-015: event_id de PED-014 reutilizado -> HTTP 409 (EVENT_ID_CONFLICT); cobrança=PENDING
T15 PED-015: 3 pedido.excluido simultâneos -> PROD-001 29->30; recusa -> 30; pedido.criado tardio -> 30
T16 PED-010: {"code":"CONFLICTING_RESULT",...} 409; PED-016: {"code":"CHECKOUT_EXPIRED",...} 409
T17: B lê/cancela PED-007 -> 404/404; SSE de B recebeu 0 eventos de PED-017
T19: GET /api/promotion-interests -> {"categories":["alimentos"],"email":"demo.promocoes@example.com"}
T20: jobs por evento: 670432b9...=1, df03a18a...=1
```

## Testes automatizados na mesma versão

`go test ./...` com `TEST_DATABASE_URL` em `ecommerce_events_test`: 182 PASS,
0 FAIL, 11 SKIP (testes de RabbitMQ). `go test ./utils/rabbitmq/` com
`TEST_RABBITMQ_URL`: 9 PASS. `npm run build`: aprovado. `git diff --check`:
aprovado.

## Dados de teste deixados no banco de desenvolvimento

Pedidos PED-007 a PED-017 do cliente A, o cliente B, 3 linhas do inbox do
Gateway em `PENDENTE_REVISAO` (eventos equivalentes injetados de propósito no
T06 e no teste da Etapa 16) e 2 jobs de e-mail `PENDING` para
`demo.promocoes@example.com` com os interesses já cancelados.

## Pendências fora desta matriz

- Envio real pelo Resend (chave do Lucas).
- Reconexão automática ao RabbitMQ (reinício manual documentado).
- Ordem invertida `pedido.enviado` × aprovação no Gateway (testes do Lucas).
