# Etapa 15 - Expiração automática do checkout

## Escopo

Implementação de Lucas na branch `feat/checkout-expiration`, sobre os serviços
de Estoque, Pagamento e Mock entregues por Gabriel. Não altera esses serviços,
nem implementa Entrega. Não requer nova migration.

## Comportamento

- O Gateway procura pedidos `AGUARDANDO_PAGAMENTO`, não excluídos, com
  `checkout_expires_at` vencido há pelo menos 30 segundos.
- A margem não prolonga a validade do pagamento: o Mock continua bloqueando
  o pagamento no prazo original. Ela apenas adia o cancelamento no Gateway.
- Resultados de pagamento já recebidos na inbox como `RECEBIDO` ou
  `PENDENTE_REVISAO` impedem a seleção do pedido para expiração.
- Uma transação bloqueia o pedido com `FOR UPDATE SKIP LOCKED`, muda seu estado
  para `CANCELADO`, registra histórico e grava `pedido.excluido` na outbox com
  motivo `CHECKOUT_EXPIRED`. Falha em qualquer gravação desfaz a transação.
- A notificação SSE é enviada somente depois do commit.
- O publicador de outbox existente envia o evento assinado; Estoque libera a
  reserva usando seu processamento idempotente. Não há acesso direto às tabelas
  de Estoque ou Pagamento pelo Gateway.
- Pedidos aprovados não são elegíveis. Um pedido já cancelado não é selecionado
  novamente e não gera outra compensação.

O executor é iniciado em `cmd/gateway/main.go`, compartilha `statusContext` e
participa do `statusWorkers`. Cada tentativa tem prazo de 10 segundos. Quando
não há trabalho ou ocorre erro, espera 5 segundos de forma cancelável. Havendo
pedidos elegíveis, continua processando um por transação.

Com serviços saudáveis, o cancelamento ocorre a partir de 30 segundos após o
vencimento, mais a espera do executor. Devolução e atualização do catálogo são
assíncronas; não prometer uma duração total exata em caso de falha ou fila acumulada.

## Arquivos

- `microservices/ms-gateway/orders/checkout_expiration.go`: operação transacional.
- `microservices/ms-gateway/orders/checkout_expiration_worker.go`: executor.
- `microservices/ms-gateway/orders/checkout_expiration_test.go`: integração isolada.
- `cmd/gateway/main.go`: inicialização e encerramento do executor.

## Demonstração

1. Manter PostgreSQL, um único RabbitMQ usado pelos serviços, Estoque, Pagamento,
   Gateway e frontend funcionando. Não usar `mock-stock` junto com Estoque real.
2. Antes de iniciar o Gateway atualizado, consultar os pedidos antigos vencidos:
   o executor também poderá cancelá-los. Conferir se são testes não pagos e
   se não há pagamentos atrasados a reconciliar.
3. No terminal do Mock, preservar `PAYMENT_WEBHOOK_SECRET` igual ao do Pagamento
   e usar validade curta antes de criar a cobrança:

```powershell
$env:MOCK_CHECKOUT_TTL = "2m"
go run ./cmd/mock-pagamento
```

Reiniciar o Mock apaga suas cobranças em memória e invalida links antigos.
Não reiniciá-lo durante o teste; criar um pedido novo depois de configurar o TTL.

4. Anotar o estoque inicial de `PROD-002`:

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce `
    -c "SELECT id, quantity FROM estoque.products WHERE id = 'PROD-002';"
```

5. Comprar uma unidade de Detergente pela interface e esperar
   `AGUARDANDO_PAGAMENTO`. A reserva deve reduzir uma unidade.
6. Não aprovar, recusar ou cancelar manualmente. Aguardar os 2 minutos de validade
   mais a margem de 30 segundos e o processamento assíncrono.
7. Confirmar `CANCELADO` sem recarregar a página e o retorno da quantidade inicial
   no catálogo/banco. O catálogo consulta novamente a cada 5 segundos quando visível.
8. Conferir a compensação no banco:

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce `
    -c "SELECT o.display_id, o.status, b.status AS evento, b.payload->>'reason' AS motivo FROM gateway.orders o JOIN gateway.event_outbox b ON b.order_id = o.id WHERE b.payload->>'reason' = 'CHECKOUT_EXPIRED' ORDER BY o.updated_at DESC LIMIT 3;"
```

Validação local: PED-039 antigo cancelado com autorização de Lucas. PED-044 novo
expirou automaticamente; Lucas confirmou atualização da tela e devolução da
unidade. Consulta confirmou `CANCELADO | PUBLICADO | CHECKOUT_EXPIRED`.
`PUBLICADO` comprova o registro da publicação; sozinho não comprova a devolução
do estoque, que deve ser observada separadamente.

## Atenção a duas instâncias de RabbitMQ

No ambiente de Lucas havia RabbitMQ nativo do Windows e RabbitMQ no Docker
usando as mesmas portas. Os serviços Go estavam conectados a `127.0.0.1:5672`
(Windows), mas `localhost:15672` mostrava o painel do Docker. O painel do broker
utilizado foi acessado por `http://127.0.0.1:15672`.

`docker compose exec rabbitmq rabbitmqctl ...` consulta exclusivamente o broker
do container, não o serviço Windows. Comparar conexões e consumidores antes de
concluir que serviços estão parados. Essa diferença não afeta as consultas ao
PostgreSQL do Docker. Não apagar filas, volumes ou parar brokers automaticamente;
padronizar o ambiente de forma coordenada fora da demonstração em andamento.

## Testes e limites

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./microservices/ms-gateway/orders -run TestExpireNextCheckoutIntegration -v -count=1
go test ./... -count=1
```

O teste recusa outro nome de banco e limpa somente seus registros. Bloqueia
temporariamente os pedidos preexistentes para que `SKIP LOCKED` não os selecione.
Não executar workers ou outras integrações simultaneamente no banco isolado.
Sem `TEST_DATABASE_URL`, a integração é pulada, não validada.

Cenários testados: vencido, prazo válido, já aprovado, pagamento pendente na
inbox e vencido recentemente. Também confere uma única entrada de histórico,
compensação `PENDENTE` com motivo correto e repetição sem nova alteração.

**Limitação conhecida:** a margem de 30 segundos e a consulta à inbox não são
uma confirmação do estado do pagamento no provedor. Um pagamento decidido antes
do prazo pode chegar somente depois do cancelamento, por indisponibilidade ou
atraso prolongado. A decisão entre serviços não é atômica; este fluxo não resolve
automaticamente cobrança aprovada tardiamente, estorno ou reconciliação. Eventos
que não puderem transicionar no Gateway podem acabar em `PENDENTE_REVISAO`.
Não demonstrar falhas prolongadas como cenário resolvido e não apresentar o fluxo
como pronto para pagamentos reais. Uma solução completa exige acordo com Gabriel
sobre a autoridade da expiração e o tratamento de resultados atrasados.

Esta etapa cobre a demonstração acadêmica com serviços saudáveis, expiração,
outbox, liberação de estoque e SSE. Entrega permanece fora do escopo.
