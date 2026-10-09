# Etapa 10 - Atualização dos pedidos por SSE

## Base

Branch feat/order-sse parte de 1b7b795, igual à main após o PR #20.
Lucas escreve o código da aplicação; o assistente consulta e revisa os arquivos,
orienta testes e mantém a documentação. Integração real com Gabriel permanece
pendente; esta etapa pode ser validada com eventos assinados simulados.

## Implementação

GET /api/orders/events autenticado, conforme etapa 02. Cada conexão recebe apenas
avisos dos pedidos do usuário autenticado, sem aceitar user_id enviado pelo cliente.
Eventos: connection.ready, connection.heartbeat, order.status.changed e
payment.checkout.available. React consulta GET /api/orders após reconectar para
sincronizar o estado atual; não haverá garantia de replay de todas as transições.

1. Central em memória por usuário, com até 16 eventos por conexão; suporta várias abas.
2. Endpoint envia e descarrega os frames imediatamente, com prazo de escrita de
   5 segundos por envio; mantém o WriteTimeout das outras rotas inalterado.
3. Heartbeat nomeado a cada 15 segundos. A conexão encerra após um minuto;
   a reconexão passa novamente pela autenticação.
4. Cancelamento e eventos de status notificam somente após o commit final.
   Eventos não aplicados, repetidos ou reagendados não geram aviso.
5. Checkout aplicado gera order.status.changed e payment.checkout.available,
   com orderId, checkoutUrl e expiresAt no segundo aviso.
6. React usa EventSource com cookies, agrupa consultas próximas por 100 ms,
   sincroniza lista e detalhes e revalida o cancelamento aberto.
7. Aviso visual fixo por 8 segundos, com largura e altura adaptadas à janela.
   Atualizações em segundo plano não escondem a lista.
8. Sem sinal por 45 segundos, verificado a cada 5 segundos, React fecha a
   conexão silenciosa e cria outra. Ao desmontar, fecha conexão e temporizadores.
9. Proxy de desenvolvimento trata encerramento incompleto da resposta SSE;
   @types/node foi adicionado para tipagem da configuração do Vite.

## Validação automatizada

- Hub: isolamento entre usuários, múltiplas conexões, unsubscribe idempotente
  e encerramento da conexão lenta sem prejudicar a ativa.
- Frames: JSON compacto, identificador opcional e rejeição de entrada inválida.
- Endpoint sem usuário autenticado responde 401.
- TestOrderSSEStreamIntegration usa autenticação real e servidor HTTP de teste:
  cabeçalhos, connection.ready, evento destinado ao usuário e limpeza ao desconectar.
- TestApplyStatusEventIntegration verifica aviso de cancelamento, ausência de
  aviso em tentativas proibidas/repetidas, eventos inválidos e duplicados,
  processamento da inbox, reagendamento e conteúdo do aviso de checkout.
- Testes de banco exigem ecommerce_events_test e limpam somente dados próprios.
  Não usar o banco da aplicação como banco de testes.

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./microservices/ms-gateway/orders -run 'TestOrderSSE|TestFormatOrderSSEEvent|TestApplyStatusEventIntegration' -v -count=1
go test ./... -count=1
Push-Location frontend
npm run build
Pop-Location
git diff --check
```

## Validação manual

- PED-037: pedido PENDENTE de R$ 4,90 recebeu o evento assinado simulado
  18de6f33-2561-49ce-8815-9ef90cbf6e3b, enviado duas vezes.
  Banco confirmou ESTOQUE_CONFIRMADO e inbox PROCESSADO; usuário observou
  atualização na tela sem clicar em Atualizar.
- Notificação inicialmente pouco perceptível: lista deixou de piscar e aviso
  passou a ser destacado por 8 segundos. Legibilidade confirmada pelo usuário.
- Após falha na detecção inicial, heartbeat nomeado e detector de ausência de
  sinais foram adicionados. Usuário confirmou reconexão após parar/reiniciar
  somente o gateway, sem recarregar a página, mantendo a conta aberta.
- Isolamento em outra conta/janela e notificação em janela reduzida foram
  confirmados pelo usuário.
- Estes testes não provam reserva/liberação real de estoque nem pagamento real.

## Limitações

A central em memória atende conexões da mesma instância do gateway. Múltiplas
instâncias exigirão fan-out entre gateways; não é garantido por esta etapa.
Se a fila de um cliente lotar, sua conexão deve encerrar para reconectar e
sincronizar por REST, em vez de bloquear o processamento dos pedidos.
Avisos em memória não substituem a persistência de pedidos/inbox/outbox.
Criação de pedido e falha de publicação ainda usam a atualização existente da
interface; não recebem avisos SSE próprios nesta etapa.
A expiração da sessão é reavaliada na reconexão, não a cada evento.
O temporizador da notificação continua contando se a aba ficar oculta.
Na abertura da aplicação, falha de consulta da sessão ainda pode mostrar a tela
de login mesmo sendo indisponibilidade temporária; esse tratamento permanece pendente.

## Estado

Implementação e testes da etapa realizados na branch feat/order-sse.
Validação final do diff, commit e PR ainda pendentes. Integração real com os
serviços de Gabriel e tela de mock de pagamento serão etapas posteriores.
