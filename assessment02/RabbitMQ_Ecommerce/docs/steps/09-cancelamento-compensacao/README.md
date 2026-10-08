# Etapa 09 - Cancelamento e compensação (em andamento)

Responsável principal: Lucas. Integração de liberação de estoque: Gabriel.

## Base confirmada

Branch feat/order-cancellation no commit 3311685, igual à main após PR #19.
Somente frontend/tsconfig.app.tsbuildinfo estava modificado no início.
Lucas implementa o código; o assistente mantém os READMEs.

## Objetivo e contrato

- DELETE /api/orders/{orderId} exige autenticação e propriedade do pedido.
- Cancelamento pelo cliente permitido em PENDENTE ou ESTOQUE_CONFIRMADO.
- Após checkout ou pagamento, retornar 409 sem alterar pedido nem estoque.
- Preservar pedido, itens e histórico; não executar exclusão física.
- Publicar pedido.excluido assinado pelo ms-principal na exchange eCommerce.
- Payload: order_id, customer_id, reason e cancelled_at em UTC.
- Motivos do contrato: CUSTOMER_CANCELLED, PAYMENT_REFUSED,
  CHECKOUT_EXPIRED e INTERNAL_FAILURE. Isso não autoriza implementar todos
  os gatilhos nesta etapa; foco em cancelamento manual e recusa de pagamento.
- Após recusa, preservar PAGAMENTO_RECUSADO e solicitar liberação de estoque.
- O Estoque trata reserva existente e cancelamento preventivo de forma idempotente.

## Estratégia

Guardar a solicitação de publicação em uma outbox na mesma transação da
mudança do pedido/histórico. Um publicador envia o envelope assinado com
identificador estável e registra a confirmação do broker. Reenvios podem ocorrer;
o consumidor de Estoque precisa ser idempotente. Confirmação do broker não é
confirmação de liberação efetiva do estoque.

Não acessar tabelas do Estoque nem marcar compensação concluída sem evidência.
Não reutilizar diretamente PublishCreated: o código atual não possui confirmações
do broker nem reconexão e não resolve atomicidade entre banco e publicação.

## Primeiro passo orientado

Lucas adicionou WebOrderDeletedPayload e constantes dos quatro motivos em
utils/events/order_web.go, preservando os tipos existentes. Apresentou gofmt e
go test ./... -count=1 aprovados. Os tipos representam o contrato, não implementam
cancelamento. Foi adicionada ValidateOrderDeletedPayload no mesmo
event_payload_reader.go, conferindo identificador do pedido, UUID do cliente,
motivo permitido e data. Lucas apresentou os quatro motivos válidos e oito
casos inválidos aprovados em TestValidateOrderDeletedPayload com -count=1.
Foi criada 005_gateway_event_outbox.sql, com identificador estável,
payload, estado da publicação e vínculo único por pedido/tipo. O envelope será
assinado uma vez e armazenado antes do primeiro envio para preservar o conteúdo
em reenvios. Lucas aplicou a migration em ecommerce e ecommerce_events_test,
com BEGIN, CREATE TABLE, CREATE INDEX e COMMIT aprovados. A inspeção confirmou
chave primária, vínculo ao pedido, unicidade pedido/tipo e estados/restrições.
A criação da tabela não implementa publicação nem compensação.
Foi criado cancellation.go com Cancel e enqueueOrderDeletedTx, conferindo
propriedade, bloqueando o pedido e persistindo CANCELADO, histórico e outbox
na mesma transação. Repetição de cancelamento retorna sem criar novo evento;
outros estados são conflito. Não excluir fisicamente nem esconder o histórico.
Lucas apresentou gofmt e go test ./... -count=1 aprovados; os testes existentes
não exercitavam Cancel. Lucas ampliou o teste de integração existente e apresentou
TestApplyStatusEventIntegration aprovado com -count=1 em ecommerce_events_test:
cancelamento de PENDENTE/ESTOQUE_CONFIRMADO, isolamento, conflito após checkout
e repetição sem duplicar histórico/outbox. A limpeza remove outbox antes dos pedidos.
Foi registrado pedido.excluido com PAYMENT_REFUSED dentro de
applyStatusEventTx após registrar a transição de recusa, preservando
PAGAMENTO_RECUSADO. Lucas apresentou TestApplyStatusEventIntegration aprovado
com -count=1 após conferir conteúdo da compensação e uma única entrada na outbox
mesmo com recusa repetida. Foi adicionado BuildOrderDeletedEvent reutilizando
o event_id da outbox e data do payload antes de assinar. Lucas apresentou
TestBuildOrderDeletedEventSignature aprovado com -count=1: campos preservados,
assinatura válida, adulteração rejeitada e rejeição de ID/chave ausentes.
Foi adicionado PrepareOutboxEnvelope no mesmo cancellation.go, bloqueando a
entrada pendente e confirmando a gravação do envelope antes da publicação.
Se já houver envelope, reutiliza-o sem gerar outra assinatura. Lucas apresentou
gofmt e go test ./... -count=1 aprovados. O novo método ainda não foi exercitado
no teste de banco. Próximo passo: ampliar o teste existente para conferir duas
preparações idênticas, assinatura válida e envelope persistido sem marcar PUBLICADO.
Na leitura de utils/cryptography/envelope.go foi identificado que a assinatura
preserva a ordem dos campos do payload. Guardar o envelope em JSONB pode alterar
essa ordem e invalidar a assinatura ao reler. Antes do teste de preparação,
converter apenas event_outbox.envelope para TEXT por uma nova migration e ajustar
o UPDATE de PrepareOutboxEnvelope para $2::text. O payload não assinado pode
continuar JSONB. Se já existirem envelopes preparados, verificar suas assinaturas:
a conversão não recupera a ordem original perdida pelo JSONB.
Lucas aplicou 006_outbox_envelope_text.sql em ecommerce e ecommerce_events_test
com sucesso. O UPDATE foi conferido com $2::text; gofmt e go test ./... -count=1
passaram. Falta exercitar PrepareOutboxEnvelope no teste de integração, incluindo
releitura e verificação da assinatura, antes de implementar o publicador.
Lucas executou TestApplyStatusEventIntegration com TEST_DATABASE_URL definido
para ecommerce_events_test e -count=1: PASS, sem SKIP. O teste ampliado confere
duas preparações idênticas, assinatura válida após releitura, texto preservado
no banco e estado PENDENTE com zero tentativas de publicação. Próximo bloco:
publicação da outbox com confirmação do RabbitMQ e tratamento de falhas.
Foram adicionados MarkOutboxPublished e ScheduleOutboxRetry no cancellation.go.
A primeira operação exige envelope persistido e registra PUBLICADO; a segunda
registra erro e agenda tentativa em 10 segundos, passando para PENDENTE_REVISAO
na décima segunda falha. Lucas apresentou gofmt e go test ./... -count=1
aprovados. As novas operações ainda precisam de teste específico de banco;
nenhuma confirmação real de publicação da outbox foi executada.
Lucas ampliou TestApplyStatusEventIntegration e apresentou PASS com -count=1
em ecommerce_events_test: erro simulado preserva o envelope, incrementa tentativa
e reagenda; registro de sucesso marca PUBLICADO, define published_at e limpa erro;
evento publicado rejeita novo registro de publicação e reagendamento. Isso valida
as operações de banco, não uma confirmação real do RabbitMQ. O limite de 12 falhas
também ainda precisa de teste específico.
Foi adicionada PublishConfirmedEvent em event_publisher.go, sem alterar
PublishCreated. Usa canal exclusivo por tentativa, mensagens persistentes,
mandatory e confirmação positiva, verificando devolução por ausência de destino.
Lucas apresentou gofmt e go test ./... -count=1 aprovados. A função ainda não
foi exercitada contra o RabbitMQ nem conectada ao processamento da outbox.
O próximo teste deve usar fila temporária e routing key exclusiva de teste,
sem emitir eventos de negócio para os serviços reais.
Lucas apresentou TestPublishConfirmedEventIntegration aprovado com -count=1
e TEST_RABBITMQ_URL definido: mensagem confirmada recebida em fila temporária,
devolução detectada para rota sem destino e rejeição de conexão ausente.
O teste usa routing key exclusiva e não aciona compensações de pedidos reais.
Próximo bloco: selecionar eventos pendentes vencidos e executar o processamento
da outbox, preservando o event_id nas repetições.
Foi adicionado ClaimNextOutboxEvent em cancellation.go. Seleciona evento PENDENTE
vencido com FOR UPDATE SKIP LOCKED e reserva a tentativa por 60 segundos no mesmo
comando SQL. Lucas apresentou gofmt e go test ./... -count=1 aprovados.
A seleção ainda precisa de teste específico; a reserva temporária permite
recuperação após interrupção, mas não oferece entrega exatamente uma vez.
Foi adicionado ProcessNextOutboxEvent: seleciona um evento, prepara e verifica
a assinatura, chama o publicador recebido e registra sucesso ou reagendamento.
Lucas apresentou gofmt e go test ./... -count=1 aprovados. A rotina ainda não
foi ativada no gateway nem exercitada por teste específico. O próximo teste usa
publicador simulado e somente compensações da conta criada no banco de testes.
Lucas apresentou PASS de TestApplyStatusEventIntegration/processamento_da_outbox:
erro simulado reagenda, publicação não é chamada antes do prazo e nova tentativa
reutiliza o mesmo envelope, marcando PUBLICADO com duas tentativas. O teste roda
em ecommerce_events_test e não publica compensações reais. Próximo bloco:
rotina contínua de publicação e ligação ao ciclo de vida do gateway.
Foi adicionada RunOutboxPublisher em event_publisher.go e ligada ao gateway como
terceira rotina, compartilhando cancelamento e espera de encerramento existentes.
Cada tentativa abre conexão própria, publica com confirmação e registra o
resultado via ProcessNextOutboxEvent. Lucas apresentou gofmt e go test ./...
-count=1 aprovados. Ainda falta executar e validar a rotina contínua: antes de
iniciar o gateway, conferir a outbox real e bindings de pedido.excluido para
evitar consumir tentativas de eventos sem fila de destino.
Lucas consultou ecommerce: outbox vazia. Criou a fila gateway.pedido-excluido.teste
e confirmou o binding eCommerce -> gateway.pedido-excluido.teste com routing key
pedido.excluido. Essa fila permite inspecionar mensagens, mas não executa liberação
de estoque. Próximo bloco: endpoint DELETE autenticado e link de cancelamento
condicional para gerar a compensação pelo fluxo HTTP.
Lucas adicionou DELETE /api/orders/{orderId}, tratamento de 401/404/409/500
e resposta 204 para cancelamento ou repetição já cancelada. addOrderLinks inclui
cancel somente em PENDENTE ou ESTOQUE_CONFIRMADO. Os arquivos foram lidos e
revisados: autenticação, filtro de proprietário, transação com histórico/outbox
e link condicional estão coerentes com a regra atual. Lucas apresentou gofmt
e go test ./... -count=1 aprovados. Ainda falta validar o endpoint via HTTP e
a publicação da compensação pela rotina contínua. Lucas pediu leitura e revisão
do código a cada atualização, além da conferência dos testes.
Validação manual em ecommerce: gateway iniciou publicador da outbox e consumidor
de status. GET de PED-033 retornou CANCELADO com itens preservados e apenas self.
Consulta ao banco confirmou uma única compensação pedido.excluido, event_id
e480c22b-4eb3-48ce-bf98-9da45322027d, PUBLICADO, attempt_count=1, published_at
preenchido e last_error vazio. Histórico contém somente PENDENTE e CANCELADO.
Isso confirma gravação e publicação pela rotina; falta inspecionar a mensagem
na fila de teste. Os códigos das duas chamadas DELETE ainda não foram enviados,
e ainda faltam validação HTTP de isolamento/conflito e ação no frontend.
Lucas inspecionou a mensagem na fila gateway.pedido-excluido.teste: event_id
igual ao da outbox, event_type pedido.excluido, producer ms-principal, order_id
PED-033, customer_id correspondente, reason CUSTOMER_CANCELLED e cancelled_at
igual ao timestamp do envelope. Assinatura está preenchida; a inspeção visual
não constitui verificação criptográfica independente da mensagem capturada.
Entrega à fila de teste confirmada, sem comprovar liberação pelo Estoque real.
Lucas confirmou via HTTP: repetição de DELETE de PED-033 pelo proprietário
retorna 204; DELETE sem sessão retorna 401; segundo cliente autenticado retorna
404 com pedido não encontrado. Falta validar 409 para estado não cancelável
e implementar a ação de cancelamento no frontend.
Lucas confirmou GET de PED-011 em FALHA_NO_PROCESSAMENTO, apenas com self;
DELETE pelo proprietário retornou 409 com mensagem de estado não cancelável.
Respostas HTTP 204/401/404/409 validadas manualmente. O caso 409 após checkout
está coberto no repositório pelo teste de banco, mas não foi repetido via HTTP.
Próximo bloco: função DELETE no cliente e botão condicionado a _links.cancel.
Frontend implementado por Lucas: cancelOrder trata DELETE/204, botão depende
de _links.cancel com método DELETE, pede confirmação e bloqueia ações durante
cancelamento/consulta de detalhes. Lista é recarregada após a tentativa e detalhes
do pedido cancelado são fechados. Código lido e revisado. A pedido de Lucas,
os botões foram agrupados em order-actions com flex-wrap e gap de 0.75rem,
mantendo as mensagens fora do grupo. npm run build executado e aprovado após
o ajuste. Falta confirmar visualmente o espaçamento e testar cancelamento na tela.
Lucas solicitou substituir window.confirm por confirmação personalizada.
Foi implementado dialog modal no próprio Orders.tsx, com título/descrição
acessíveis, foco inicial em Voltar, contenção de foco nativa e fechamento via Esc.
Voltar não envia DELETE; Confirmar cancelamento fecha o diálogo e executa o
fluxo existente. Ref impede chamadas duplicadas enquanto a requisição está ativa.
CSS segue os tokens do projeto, com fundo escurecido e botões responsivos.
npm run build aprovado. Inspeção visual e teste manual de teclado/cancelamento
do diálogo ainda pendentes; nenhum pedido foi cancelado durante a implementação.
Lucas apresentou captura de PED-008 CANCELADO na tela, preservado na lista e
sem botão de cancelamento. Consulta ao banco confirmou uma única compensação
PUBLICADO, attempt_count=1 e last_error vazio. Voltar/Esc ainda não foram
confirmados pelo usuário. Próximo teste: cancelar um pedido de teste com RabbitMQ
temporariamente parado e confirmar publicação automática após sua retomada.
A integração com os serviços reais de Gabriel continua pendente por ainda não
estarem prontos; os testes desta etapa usam a fila de captura.
Teste real de indisponibilidade: após criar PED-036 PENDENTE, Lucas parou somente
RabbitMQ, cancelou por HTTP (204) e observou CANCELADO com outbox PENDENTE,
attempt_count=1 e erro de conexão recusada. RabbitMQ foi religado no finally.
A consulta posterior mostrou evento 7f79a37e-c408-4cc4-9f7d-58b7583fcea3 PUBLICADO,
attempt_count=3, published_at preenchido e last_error vazio, no teste orientado
a manter o gateway aberto. Recuperação da outbox observada; ainda falta teste
determinístico do limite de 12 falhas. O publicador antigo de pedido.criado não
reconecta: PED-034/PED-035 registraram canal fechado; gateway foi reiniciado antes
da criação de PED-036. Essa limitação não é corrigida pela outbox de compensação.

Lucas apresentou PASS de TestApplyStatusEventIntegration e dos subtestes de
processamento e limite de falhas. O último bloco foi lido e revisado: usa somente
compensação da conta de teste, mantém o envelope, verifica tentativas 1 a 11
PENDENTE e tentativa 12 PENDENTE_REVISAO, sem published_at, e rejeita novo
reagendamento/registro de publicação nesse estado.

## Pendências antes do PR

1. Confirmar manualmente Voltar e Esc na janela de confirmação.
2. Revisar arquivos a incluir no commit, excluindo artefatos de build e chaves.

Lucas apresentou go test ./... -count=1 e npm run build aprovados na validação
final. git diff --check apresentou apenas avisos LF/CRLF, sem erros de whitespace.
Os testes específicos de banco e RabbitMQ já tiveram PASS sem SKIP nesta etapa.
frontend/tsconfig.app.tsbuildinfo permanece fora do commit por ser gerado.

## Integração posterior

Integração e liberação de reserva no Estoque real dependem da implementação de
Gabriel. A fila de teste comprova recepção, não execução da compensação.
O publicador antigo de pedido.criado ainda não possui reconexão automática.

## Validação

Tipos do contrato, assinatura e cancelamento no banco de testes validados.
Preparação persistida do envelope e reutilização da assinatura validadas no
teste de integração. Endpoint validado manualmente com 204/401/404/409;
publicação observada na fila de captura e recuperação após queda do RabbitMQ
validada. Limite de 12 falhas validado no banco isolado. Cancelamento pela tela
observado para PED-008. Integração real com Estoque ainda não validada.
Não incluir chaves privadas nem artefatos de build nos commits.
