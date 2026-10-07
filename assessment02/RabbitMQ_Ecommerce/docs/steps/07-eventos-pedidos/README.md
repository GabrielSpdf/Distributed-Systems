# Etapa 07 - Eventos de pedidos (em andamento)

Responsável principal: Lucas.

Branch atual: `feat/gateway-order-events`.

## Objetivo

Publicar `pedido.criado` pelo Gateway após persistir o pedido, reutilizando o envelope e a assinatura digital do projeto anterior.

## Primeiro passo

Foi adicionado `utils/events/order_web.go` com os tipos `WebOrderItem` e `WebOrderCreatedPayload`.

O payload segue o contrato aprovado na Etapa 02: identificador do pedido, identificador do cliente, itens, total e moeda.

## Teste de serialização

Foi criado `utils/events/order_web_test.go`, com o teste
`TestWebOrderCreatedPayloadJSON`. Lucas apresentou sua execução aprovada.

O teste compara o JSON gerado com o contrato, conferindo nomes dos campos,
valores, itens e moeda. A comparação considera o conteúdo, sem depender da
ordem dos campos, espaços ou representação decimal equivalente.
Também detecta campos extras. Não valida a autenticidade do evento nem o
formato do identificador do cliente: essas verificações pertencem a outros testes.

Após criar o arquivo, executar na raiz do projeto:

```powershell
gofmt -w utils/events/order_web_test.go
go test ./utils/events -v
```

## Compatibilidade

Os tipos antigos permanecem inalterados. Isso preserva o código anterior, mas não torna os consumidores antigos compatíveis com o novo payload.

Gabriel deverá adaptar o consumidor do Estoque ao contrato web. Alterações em arquivos compartilhados devem ser coordenadas pela dupla.

## Pendências

- validar o recebimento e processamento pelo Estoque com Gabriel;
- confirmar que as chaves privadas estão ignoradas pelo Git.

A publicação existente ainda não utiliza confirmação do broker. Retornar sem erro não comprova que o Estoque recebeu ou processou a mensagem.

## Validação

Tipos adicionados e formatados. Lucas apresentou `go test ./...` aprovado
após essa alteração, sem falhas nos pacotes existentes.

Teste específico de serialização: `TestWebOrderCreatedPayloadJSON` aprovado
com `go test ./utils/events -v`.

## Próximo passo orientado

Foi implementado `microservices/ms-gateway/orders/event_payload.go` com a função
`BuildOrderCreatedPayload`, que converte o pedido persistido para o contrato web.
Ela usa o identificador público em order_id e UserID em customer_id, copiando os
itens, o total calculado pelo backend e a moeda do pedido, sem incluir links
HTTP ou o UUID interno do pedido. Lucas apresentou `go test ./...` aprovado
após adicionar essa função.

O próximo passo é criar `microservices/ms-gateway/orders/event_payload_test.go`
com `TestBuildOrderCreatedPayload`, usando dois itens e identificadores interno
e público diferentes para conferir o mapeamento. Lucas implementou o teste e
apresentou `go test ./microservices/ms-gateway/orders -v` aprovado.

Foi criado `event_envelope.go` no pacote orders com
`BuildOrderCreatedEvent`, reutilizando `misc.BuildSignedEnvelope`, o tipo
`pedido.criado` e o produtor `ms-principal`. A função monta e assina; ainda
não publica no RabbitMQ.

Lucas apresentou `TestBuildOrderCreatedEventSignature` aprovado, junto aos
demais testes de orders. O teste gera uma chave RSA temporária na memória,
verifica a assinatura original e confirma a rejeição após alterar o payload.
Nenhuma chave real é modificada pelo teste.

O event_id foi alinhado ao formato UUID v4 em `utils/misc/misc.go`, com
verificação de formato no teste de assinatura. Lucas apresentou
`go test ./...` aprovado após o ajuste. O helper é compartilhado; Gabriel deve
ser informado dessa alteração antes da integração.

Foi criado `event_publisher.go` no pacote orders com
`EventPublisher`, reutilizando `rabbitmq.PublishEvent`. O método PublishCreated
monta o evento assinado e o envia à exchange eCommerce com routing key
pedido.criado. Um mutex serializará as publicações feitas pela mesma instância.
Lucas apresentou `go test ./...` aprovado após implementar o publicador.

Foi aberta a conexão RabbitMQ e seu canal em
`cmd/gateway/main.go`, declarando a exchange eCommerce como direct e fechando os
recursos com defer. Lucas apresentou inicialização bem-sucedida do Gateway
com conexão ao RabbitMQ, PostgreSQL e servidor HTTP na porta 8080.

Lucas executou `go run ./cmd/keygen` com sucesso, gerando os pares locais de
Principal, Estoque, Pagamento e Entrega e distribuindo suas chaves públicas.
Coordenar as chaves públicas com Gabriel para a integração. Não expor nem
versionar chaves privadas, nem sobrescrever chaves existentes.

Próximo passo orientado: carregar `keys/ms-principal/private.pem` no Gateway,
criar EventPublisher e passá-lo como terceiro argumento ao handler de pedidos.
O handler guarda a dependência; a chamada efetiva após a persistência e o
tratamento de falhas serão implementados depois. Lucas apresentou o Gateway
iniciando com sucesso após essa configuração. A verificação de private.pem
ignorado pelo Git ainda precisa ser apresentada por Lucas.

Antes da primeira publicação, preparar no painel RabbitMQ uma fila durável
`gateway.pedido-criado.teste`, sem consumidores, ligada à exchange eCommerce
pela routing key pedido.criado. Essa fila serve somente para inspeção local;
não substitui a fila nem o consumidor do MS Estoque. Lucas confirmou sua
preparação no painel. Binding e recebimento serão conferidos no teste de
publicação. Não iniciar consumidores antigos com o payload web incompatível.

Foi implementado MarkProcessingFailed no repositório,
gravando FALHA_NO_PROCESSAMENTO e histórico em uma transação somente se o
pedido ainda estiver PENDENTE. O método usa o UUID interno para atualizar o
banco; o evento continua usando o identificador público. Lucas apresentou
`go test ./...` aprovado. A alteração real no banco ainda precisa ser testada.
Essa marcação não equivale a provar que uma mensagem não chegou ao
broker e não autoriza republicação automática; falhas podem ser ambíguas.

A criação de pedidos já chama o publicador após persistir o pedido.

Foi orientada a chamada de PublishCreated após Create confirmar a transação.
Verificar a dependência antes de persistir. Em erro de publicação, tentar
registrar a falha com contexto próprio limitado a cinco segundos e atualizar
a resposta somente se a gravação funcionar. Manter HTTP 201 porque o recurso
foi criado; não republicar nem criar outro pedido automaticamente. Ajustar a
mensagem do frontend para distinguir falha de processamento. O publicador
atual não confirma aceitação pelo broker nem entrega na fila; essas garantias
não podem ser inferidas de um retorno sem erro. O caminho de sucesso foi
validado pela criação do pedido e presença de mensagem na fila de teste.

## Primeiro teste de publicação

Lucas apresentou login HTTP 200 e criação HTTP 201 do PED-010, com uma unidade
de PROD-002 (Detergente), total R$ 4,90, moeda BRL e status PENDENTE.
A listagem das filas mostrou gateway.pedido-criado.teste com uma mensagem
pronta e zero consumidores. Lucas apresentou o JSON obtido na inspeção:
event_id 82c0d291-3fca-42ed-a10b-9f604e57bfcf, tipo pedido.criado, produtor
ms-principal e payload com order_id PED-010, cliente correto, uma unidade de
Detergente a R$ 4,90, total 4,9 e moeda BRL. A assinatura estava preenchida.
O conteúdo recebido está alinhado ao contrato. A inspeção visual não verifica
criptograficamente essa assinatura; isso foi exercitado pelos testes unitários,
e deverá ser validado também pelo consumidor real.

O Estoque real ainda não processou esse pedido. Foi orientada a inspeção com
requeue habilitado para preservar a mensagem de teste.

Próximo ajuste orientado no frontend: em handleCreateOrder, distinguir o
status FALHA_NO_PROCESSAMENTO na confirmação, mantendo a limpeza do carrinho
e a atualização da lista porque o pedido já foi salvo. Lucas apresentou
`npm run build` aprovado após o ajuste. A confirmação normal orienta a
consultar Meus pedidos, sem prometer que o processamento foi concluído.

## Teste controlado de falha

Manter Gateway, PostgreSQL e mock-stock em execução, com sessão HTTP válida.
Parar somente o contêiner RabbitMQ e criar um único pedido. Esperar HTTP 201
com FALHA_NO_PROCESSAMENTO e conferir posteriormente o estado e o histórico
no PostgreSQL. Não repetir a criação se ocorrer erro sem primeiro consultar
o banco. Reiniciar RabbitMQ em finally, sem apagar volumes, aguardar healthy
e reiniciar o Gateway: ainda não há reconexão automática do canal.
O teste não deve ser executado durante uso dos serviços por Gabriel.

Lucas executou o teste: após parar somente RabbitMQ, o Gateway registrou erro
de canal/conexão fechado na publicação do PED-011. A resposta foi HTTP 201
com status FALHA_NO_PROCESSAMENTO, uma unidade de Detergente, total R$ 4,90
e updated_at posterior a created_at. Não foi criado outro pedido como retry.

RabbitMQ foi iniciado novamente e apresentado como healthy, assim como
PostgreSQL. Lucas reiniciou o Gateway e apresentou inicialização bem-sucedida.
Lucas confirmou diretamente no PostgreSQL o PED-011 com total 4,90 e estado
FALHA_NO_PROCESSAMENTO, além das duas entradas no histórico: PENDENTE e
FALHA_NO_PROCESSAMENTO, nessa ordem. O caminho de falha foi validado no banco.
O erro 504 exibido no log é do cliente AMQP; não é o status HTTP da resposta.

## Conflito RabbitMQ no Windows e Docker

Foi identificado um serviço RabbitMQ do Windows em execução na porta 5672.
O Gateway conectava ao serviço local, enquanto a inspeção era feita no
contêiner, que não apresentava conexões nem a exchange eCommerce.

A solução orientada foi parar temporariamente o serviço do Windows em um
PowerShell como administrador, reiniciar o contêiner RabbitMQ e reiniciar o
Gateway usando RABBITMQ_URL=amqp://guest:guest@127.0.0.1:5672/.
Nenhum volume ou dado foi apagado.

Lucas confirmou a solução apresentando uma conexão guest no virtual host /
do contêiner e a exchange eCommerce do tipo direct. A publicação na fila de
teste foi validada posteriormente; o consumo pelo Estoque segue pendente.

Para diagnosticar, manter o Gateway em execução e usar:

```powershell
docker compose exec -T rabbitmq rabbitmqctl list_connections name user vhost
docker compose exec -T rabbitmq rabbitmqctl list_exchanges -p / name type
```

Evitar executar simultaneamente o serviço Windows e o contêiner nas mesmas
portas. Se o serviço local for necessário, combinar portas diferentes.
