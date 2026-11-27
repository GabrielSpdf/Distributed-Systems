# Event Market — Avaliação 03

Sistema distribuído de e-commerce desenvolvido como evolução da versão web
REST da Avaliação 02 na disciplina de Sistemas Distribuídos da UTFPR.

O frontend em React/TypeScript usa GraphQL sobre HTTP para autenticação,
catálogo, pedidos e preferências de promoções. As subscriptions `pedidoAtualizado`
e `promocaoDisponivel` entregam notificações via WebSocket. Os microsserviços em Go
processam eventos pelo RabbitMQ e persistem dados no PostgreSQL. O fluxo de
pagamento usa REST e webhook com o Mock de Pagamento.

[Voltar ao repositório](../../README.md) · [Consultar a Avaliação 02](../../assessment02/RabbitMQ_Ecommerce/README.md)

## Funcionalidades

- Cadastro, login, consulta de sessão e logout via GraphQL, com cookie de sessão.
- Listagem e detalhes dos próprios pedidos.
- Catálogo consultado pelo frontend no Gateway e pelo Gateway no MS Estoque via GraphQL.
- Carrinho, criação de pedidos, reserva de produtos e consulta do histórico.
- Checkout em nova aba com aprovação ou recusa simulada.
- Expiração de cobranças, cancelamento e devolução de estoque.
- Nota fiscal e rastreamento simulados após pagamento aprovado.
- Notificações de estoque, checkout, pagamento e envio pela subscription `pedidoAtualizado`.
- Registro e cancelamento de interesses em promoções por categoria.
- Ofertas assinadas no RabbitMQ e entregues por `promocaoDisponivel` a usuários com interesse ativo na categoria.

## Arquitetura

O Gateway recebe as operações do frontend, consulta o catálogo no MS Estoque e
entrega as notificações de pedidos e promoções por WebSocket em `/graphql`.

| Componente | Responsabilidade | Comunicação |
|---|---|---|
| Frontend | Catálogo, carrinho, pedidos, preferências e notificações | GraphQL sobre HTTP para autenticação, catálogo, pedidos e preferências. GraphQL sobre WebSocket para atualizações de pedidos e ofertas. |
| Gateway | Autenticação, pedidos, outboxes, inbox e notificações por cliente | GraphQL com Estoque e eventos RabbitMQ |
| Estoque | Catálogo persistido, reserva e devolução | GraphQL interno e RabbitMQ |
| Pagamento | Cobranças, webhook, decisão e expiração | REST com Mock e RabbitMQ |
| Mock de Pagamento | Checkout e escolha de aprovação/recusa | REST e webhook HTTP POST |
| Entrega | Nota fiscal e despacho simulados | RabbitMQ |
| Promoções | Consumo de interesses e publicação de ofertas assinadas | RabbitMQ |

PostgreSQL é uma única instância local com schemas de responsabilidade separados:
`gateway`, `estoque`, `pagamento`, `entrega` e `promocoes`.
Os serviços não precisam consultar diretamente as tabelas uns dos outros no fluxo normal.

Sequência de um pedido:

1. O frontend chama `criarPedido`. O Gateway grava o pedido e o evento na outbox na mesma transação.
2. Publicador da outbox envia `pedido.criado`; Estoque verifica e reserva os itens.
3. Estoque publica `pedido.estoque_ok` ou `estoque.indisponivel`.
4. Pagamento solicita a cobrança ao Mock e publica a disponibilidade do checkout.
5. Usuário abre o checkout em nova aba; Mock envia a decisão ao webhook de Pagamento.
6. Pagamento publica aprovação ou recusa. Aprovação permite despacho pela Entrega.
7. O Gateway consome os resultados, atualiza o banco e publica a mudança de status em `pedidoAtualizado` após o commit.
8. Recusa ou expiração leva ao cancelamento e à compensação do estoque.

As mensagens são publicadas em exchanges usando routing keys; o RabbitMQ
encaminha cada evento às filas com bindings compatíveis.

### Exchanges

| Exchange | Tipo | Uso |
|---|---|---|
| `eCommerce` | `direct` | Pedidos, resultados e interesses |
| `Promocoes` | `topic` | Ofertas por categoria |

### Filas

| Fila | Routing keys |
|---|---|
| `estoque` | `pedido.criado`, `pedido.excluido` |
| `pagamento` | `pedido.estoque_ok` |
| `entrega` | `pagamento.aprovado` |
| `gateway.pedidos.status` | `pedido.estoque_ok`, `estoque.indisponivel`, `pagamento.checkout_disponivel`, `pagamento.aprovado`, `pagamento.recusado`, `pedido.enviado` |
| `promocoes.interesses` | `interesse.promocao` |
| `gateway.promocoes.*` | `promocao.categoria.*` |

Cada instância do Gateway declara uma fila exclusiva e temporária com o prefixo
`gateway.promocoes.`. Ela recebe as ofertas enquanto a conexão com o RabbitMQ
está ativa. As categorias são `alimentos`, `limpeza` e `eletronicos`.
Filas legadas `principal` e `consumidor.01/02` não são necessárias para o fluxo web.
Elas podem acumular cópias se continuarem vinculadas sem consumidor: isso não
representa automaticamente falha das filas do fluxo web. Não as purgue sem avaliar os dados.

A topologia de pedidos é centralizada em `utils/rabbitmq/ecommerce_topology.go`.
O preparo idempotente declara filas duráveis e bindings sem apagá-los.
Atributos incompatíveis produzem erro; não se substituem filas automaticamente.

### Eventos do e-commerce

| Routing key | Produtor | Consumidores do fluxo web |
|---|---|---|
| `pedido.criado` | Gateway (identidade Principal) | Estoque |
| `pedido.excluido` | Gateway (identidade Principal) | Estoque |
| `pedido.estoque_ok` | Estoque | Gateway e Pagamento |
| `estoque.indisponivel` | Estoque | Gateway |
| `pagamento.checkout_disponivel` | Pagamento | Gateway |
| `pagamento.aprovado` | Pagamento | Gateway e Entrega |
| `pagamento.recusado` | Pagamento | Gateway |
| `pedido.enviado` | Entrega | Gateway |
| `interesse.promocao` | Gateway (identidade Principal) | Promoções |

### Eventos de promoção

| Routing key | Categoria |
|---|---|
| `promocao.categoria.limpeza` | Limpeza |
| `promocao.categoria.alimentos` | Alimentos |
| `promocao.categoria.eletronicos` | Eletrônicos |
| `promocao.categoria.*` | Binding para todas as categorias |

## Estrutura do projeto

```text
RabbitMQ_Ecommerce/
├── cmd/
│   ├── gateway/
│   ├── estoque/
│   ├── pagamento/
│   ├── entrega/
│   ├── mock-pagamento/
│   ├── promocoes/
│   ├── publish-promotions/
│   ├── reprocessar/
│   └── keygen/
├── frontend/
│   └── src/
├── microservices/
│   ├── ms-gateway/
│   ├── ms-estoque/
│   ├── ms-pagamento/
│   ├── ms-entrega/
│   ├── ms-promocoes/
│   └── mock-pagamento/
├── migrations/
├── scripts/
├── data/
├── keys/                 # Local; não versionada
├── utils/
│   ├── config/
│   ├── cryptography/
│   ├── database/
│   ├── events/
│   ├── rabbitmq/
│   └── webhook/
├── .env.example
├── docker-compose.yml
├── go.mod
├── go.sum
└── README.md
```

A árvore destaca os componentes do fluxo web; comandos e arquivos legados
também permanecem na base do projeto.

Os diretórios de `cmd/` são os pontos de entrada. As regras ficam em
`microservices/`; contratos e recursos compartilhados ficam em `utils/`.
`frontend/src/` reúne telas, cliente GraphQL por HTTP e WebSocket e estilos.
`migrations/` contém as alterações SQL 001 a 016 e `scripts/` contém
o inicializador Windows. O gerador de ofertas ainda usa `data/inventory.json`.

## Tecnologias

- Go, na versão declarada em `go.mod`.
- React, TypeScript e Vite.
- RabbitMQ e protocolo AMQP 0-9-1.
- Biblioteca `github.com/rabbitmq/amqp091-go`.
- PostgreSQL 17 e driver `github.com/jackc/pgx/v5`.
- GraphQL sobre HTTP com `github.com/graph-gophers/graphql-go`.
- GraphQL Subscriptions com `graphql-transport-ws` no Gateway e `graphql-ws` no frontend.
- REST/JSON e webhook HTTP para a integração com o Mock de Pagamento.
- RSA-PSS com SHA-256 para os eventos RabbitMQ.
- HMAC-SHA-256 para autenticação do webhook.
- JWT e bcrypt para autenticação de usuários.
- Docker Compose e PowerShell para o ambiente local.

## Pré-requisitos

Execute os comandos nesta pasta, que contém `go.mod` e `docker-compose.yml`.

- Go compatível com a versão declarada em `go.mod`.
- Node.js/npm compatíveis com o frontend e suas dependências.
- Docker Desktop aberto para executar PostgreSQL 17.
- RabbitMQ nativo do Windows instalado, serviço `RabbitMQ` e painel de gerenciamento habilitado.
- Windows PowerShell 5.1 ou superior.

Não execute os brokers nativo e Docker simultaneamente nas mesmas portas.
O inicializador foi feito para PostgreSQL no Docker e RabbitMQ nativo.
Não use `docker compose up -d` sem especificar o serviço nesse ambiente.

## Preparação

A partir da raiz do repositório:

```powershell
cd assessment03/RabbitMQ_Ecommerce
```

### Dependências

```powershell
go mod download
go mod verify

Push-Location frontend
npm ci
Pop-Location
```

### Configuração e segredos

Na primeira configuração, copie `.env.example` para `.env` somente se este ainda não existir.
Nunca sobrescreva uma configuração existente por engano.
O inicializador lê `.env` como valores literais; variáveis do terminal têm prioridade.
Executar os programas Go diretamente não equivale a carregar automaticamente esse arquivo.

Configurações principais:

| Variável | Finalidade |
|---|---|
| `DATABASE_URL` | PostgreSQL local, banco `ecommerce`, porta 5432 |
| `RABBITMQ_URL` | RabbitMQ local, porta 5672 |
| `JWT_SECRET` | Assinatura de sessões; mínimo 32 bytes |
| `PAYMENT_WEBHOOK_SECRET` | Mesmo segredo no Mock e Pagamento; mínimo 32 bytes |
| `STOCK_SERVICE_URL` | `http://localhost:8081` |
| `MOCK_PAYMENT_URL` | `http://localhost:8083` |
| `PAYMENT_WEBHOOK_URL` | `http://localhost:8082/webhooks/payments` |
| `MOCK_CHECKOUT_TTL` | Prazo do checkout, por exemplo `2m` ou `30m` |
| `FRONTEND_ORIGIN` | Origem autorizada do navegador, `http://localhost:5173` no ambiente local |

Use segredos aleatórios próprios, não os exemplos públicos.
O JWT assina sessões, o segredo do webhook autentica o retorno do Mock e as chaves RSA assinam eventos.
Preserve o JWT para manter sessões após reinício; alterá-lo exige novo login.
Não publique `.env`, chaves privadas ou valores de segredos nos logs.

Segredos ausentes são pedidos com entrada oculta pelo inicializador.
Um segredo já configurado, mas curto, é rejeitado: ajuste o ambiente ou `.env`.
Comentários ao final de valores e interpolação de variáveis não são suportados pelo leitor de `.env`.

## Geração e distribuição das chaves

Em instalação nova, sem a pasta `keys`:

```powershell
go run ./cmd/keygen
go run ./cmd/keygen -promotions-only
```

O primeiro gera pares de Principal, Estoque, Pagamento e Entrega e distribui chaves públicas.
O segundo acrescenta o par de Promoções. Não regenere nem apague chaves já utilizadas
apenas para tentar resolver um erro; eventos pendentes podem depender delas.
O Gateway usa a identidade criptográfica de Principal no fluxo web.

## Assinatura digital

Todas as mensagens de negócio RabbitMQ do fluxo atual usam RSA-PSS com SHA-256.
A assinatura cobre ID, tipo, produtor, instante e payload do envelope.
O consumidor verifica assinatura, produtor autorizado e compatibilidade com a routing key.
Assinatura não cifra o conteúdo: ela verifica autenticidade e integridade.

Publicações confirmadas usam mensagens persistentes, `mandatory` e publisher confirms.
A confirmação do broker atesta a publicação roteada. A entrega ao navegador depende do Gateway e da assinatura ativa.
Para eventos distribuídos a mais de uma fila, preparar todos os bindings é necessário:
`mandatory` sozinho só detecta ausência de qualquer rota.

O sistema combina outboxes, inbox, confirmação manual e idempotência.
A garantia é processamento tolerante a reentregas, não entrega exatamente uma vez.
Envelopes persistidos mantêm identidade e assinatura nas novas tentativas.

Falhas transitórias podem ser retentadas; resultados pendentes são recuperados por workers.
Alguns consumidores usam filas `<fila>.revisao` após esgotar tentativas.
Outboxes/inbox também podem ficar em `PENDENTE_REVISAO` no banco.
Filas de revisão e registros de revisão SQL são mecanismos diferentes.
Não reenvie mensagens ou altere status manualmente sem diagnosticar a causa.

## Persistência e migrations

O fluxo web usa PostgreSQL para pedidos, reservas, cobranças, entregas,
interesses em promoções. O estoque real é persistido no PostgreSQL.
Os schemas separam a responsabilidade de cada serviço.

```powershell
docker compose up -d postgres
docker compose ps postgres
```

Em volume novo, PostgreSQL executa as migrations montadas em
`/docker-entrypoint-initdb.d` na inicialização.
Em volume existente, subir o container não aplica migrations novas.

Confira o schema e aplique somente as migrations pendentes, em ordem e com
`ON_ERROR_STOP=1`. Não reaplique todas indiscriminadamente nem remova o volume.
Exemplo para um banco existente que ainda precisa da migration 016:

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce -v ON_ERROR_STOP=1 -f /docker-entrypoint-initdb.d/016_business_input_snapshots.sql
```

A 016 acrescenta os snapshots de entrada em Estoque e Entrega, sem apagar dados.
Reservas/entregas antigas sem snapshot não permitem comprovar igualdade de novas
entradas; reentregas são rejeitadas nesse caso. Envelopes de saída armazenados
continuam recuperáveis.

## Execução

### Inicialização no Windows

Feche os programas anteriores com Ctrl+C antes de iniciar.
O script não instala dependências, gera chaves, aplica migrations ou encerra programas à força.

```powershell
# Conferência sem iniciar serviços nem solicitar segredos.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -Verificar

# Fluxo web com publicação manual de ofertas.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -AbrirTerminais

# Inclui ofertas periódicas.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -AbrirTerminais -PublicarOfertas -Categoria alimentos -Intervalo 60s
```

A inicialização padrão abre sete terminais para Estoque, Mock, Pagamento,
Entrega, Interesses, Gateway e Frontend. O script aguarda os serviços HTTP e os
consumidores das filas, incluindo `gateway.promocoes.*`. Com `-PublicarOfertas`,
o publicador periódico é aberto depois que os consumidores estão ativos.

Se o RabbitMQ do mesmo Compose estiver ativo, o script para somente esse
container, preservando volumes, para usar o broker nativo.
Se precisar iniciar o serviço Windows sem permissão suficiente, solicita que
você o inicie manualmente como administrador; não há elevação automática.

O checkout usa 2 minutos como padrão do script; `MOCK_CHECKOUT_TTL` do
ambiente/`.env` prevalece, a menos que `-ValidadeCheckout` seja passado explicitamente.
Exemplo: `-ValidadeCheckout 30m`.

| Acesso | Endereço |
|---|---|
| Frontend | http://localhost:5173 |
| Gateway | http://localhost:8080 |
| Queries e mutations do Gateway | `POST http://localhost:8080/graphql` |
| Subscriptions do Gateway | `ws://localhost:8080/graphql` via `ws://localhost:5173/graphql` no frontend local |
| Catálogo interno do Estoque | `POST http://localhost:8081/graphql` |
| Pagamento | http://localhost:8082 |
| Mock | http://localhost:8083 |
| Painel RabbitMQ nativo | http://127.0.0.1:15672 |

O script recusa configurações fora das portas/URLs locais previstas.
Se falhar após abrir alguns terminais, eles ficam para diagnóstico: pare-os
antes de repetir. Não há rollback automático dos processos já iniciados.
Para encerrar, Ctrl+C em cada terminal; banco e RabbitMQ permanecem ativos.
Não reinicie o Mock durante um checkout: as cobranças do Mock são em memória.

Sem o inicializador, configure o ambiente de cada terminal e execute:

```powershell
go run ./cmd/estoque
go run ./cmd/mock-pagamento
go run ./cmd/pagamento
go run ./cmd/entrega
go run ./cmd/promocoes -interests
go run ./cmd/gateway
# Em outro terminal, dentro de frontend:
npm run dev
```

Cada comando acima é um processo separado. Principal e consumidores legados
1/2 não são necessários; o menu legado de pedidos usa contratos antigos.

## API do Gateway

### Queries e mutations

Envie a operação em `POST /graphql` com `Content-Type: application/json`.
O corpo da requisição contém `query` e, quando necessário, `variables`.
O login envia `Set-Cookie` com o cookie `session`, marcado como `HttpOnly`.
O frontend envia o cookie nas operações autenticadas e o logout o remove.

| Tipo | Operação | Sessão | Uso |
|---|---|---|---|
| Mutation | `cadastrarUsuario(name, email, password)` | Pública | Cria uma conta |
| Mutation | `entrar(email, password)` | Pública | Valida as credenciais e inicia a sessão |
| Query | `usuarioAtual` | Autenticada | Retorna o usuário da sessão atual |
| Mutation | `sair` | Pública | Encerra a sessão no navegador, se houver |
| Query | `produtosDisponiveis(category, available)` | Pública | Consulta o catálogo no MS Estoque via GraphQL |
| Query | `pedidos` | Autenticada | Lista os próprios pedidos |
| Query | `pedido(id)` | Autenticada | Consulta os detalhes de um pedido do usuário |
| Query | `interessesPromocao` | Autenticada | Consulta as categorias ativas |
| Mutation | `criarPedido(items)` | Autenticada | Persiste o pedido e o evento na outbox |
| Mutation | `cancelarPedido(id)` | Autenticada | Cancela um pedido próprio dentro da janela permitida |
| Mutation | `registrarInteressePromocao(categories, email)` | Autenticada | Registra interesses por categoria |
| Mutation | `cancelarInteressePromocao(category)` | Autenticada | Desativa o interesse em uma categoria |

Na consulta de catálogo, omitir `available` retorna produtos disponíveis.
Envie `null` para incluir todos ou `false` para consultar os indisponíveis.
Nas mutations de promoções, `changed` indica se a preferência foi alterada.
O campo `email` em `interessesPromocao` e o argumento opcional `email` de
`registrarInteressePromocao` permanecem no schema por compatibilidade. A tela
usa apenas as categorias.

IDs internos de pedidos são UUIDs. Use o identificador de exibição `PED-...`
em `pedido`, `cancelarPedido` e no campo `orderId` da notificação.

O retorno de `data.criarPedido` confirma a persistência do pedido e do evento.
A reserva de estoque e o pagamento são processados depois pelo fluxo assíncrono.
Repetir a mutation pode criar outro pedido.

O cancelamento manual só é permitido em `PENDENTE`, antes de preparar o envelope
de criação. A transação retira essa intenção ainda não enviada e grava a compensação.
Se o processamento já avançou, a mutation retorna um erro de conflito no campo
`errors` da resposta GraphQL. O pedido, os itens e o histórico permanecem no banco.

Pagamento é a única autoridade de expiração. A cobrança vencida gera
`pagamento.recusado` com motivo `CHECKOUT_EXPIRED`; Gateway cancela e publica
`pedido.excluido` para compensar. O antigo worker de expiração do Gateway não
é iniciado em `cmd/gateway`. Aprovação e expiração disputam a mesma cobrança
sob controle transacional, evitando decisões locais independentes.

### Atualizações em tempo real

O frontend abre um WebSocket em `/graphql` com o subprotocolo
`graphql-transport-ws`. O Vite encaminha a conexão ao Gateway no ambiente local.
O Gateway verifica a origem configurada e usa o cookie `session` para identificar
o usuário. A assinatura recebe somente atualizações dos pedidos desse usuário.

```graphql
subscription OrderUpdates {
  pedidoAtualizado {
    eventId
    orderId
    status
  }
}
```

O Gateway publica as mudanças de status após o commit. O frontend usa `eventId`
para evitar avisos duplicados enquanto estão visíveis. Cada aviso dura oito
segundos. Quando o pedido passa a `AGUARDANDO_PAGAMENTO`, a tela consulta
`pedidos` para carregar o checkout. O Mock continua recebendo chamadas REST e
enviando o resultado pelo webhook de Pagamento.

Ao reconectar, o frontend refaz a query `pedidos` para recuperar o estado atual.
Os avisos ocorridos durante a desconexão podem se perder.

## Promoções

O Gateway grava interesses e cancelamentos por categoria e publica
`interesse.promocao` pela outbox. O MS Promoções consome esses eventos da fila
`promocoes.interesses` e atualiza seu registro de interesses. O publicador gera
ofertas a partir de `data/inventory.json` e publica eventos assinados na exchange
`Promocoes`, com uma routing key por categoria.

O Gateway valida assinatura e categoria, recebe a oferta na fila temporária
`gateway.promocoes.*` e consulta os interesses ativos do usuário na hora da
entrega. Registrar ou cancelar um interesse passa a valer para as próximas
ofertas na mesma conexão. Na interface, **Ofertas em tempo real** mostra até as
dez ofertas recebidas mais recentes e evita repetir um `eventId` presente na lista.

```graphql
subscription PromotionOffers {
  promocaoDisponivel {
    eventId
    productId
    productName
    category
    discountPercentage
    originalPrice
    promotionalPrice
  }
}
```

Com o Gateway conectado ao RabbitMQ, publique manualmente a partir da pasta da
Avaliação 03. Em um terminal separado, configure `RABBITMQ_URL` quando for
necessário apontar para o mesmo broker usado pelo Gateway.

```powershell
go run ./cmd/publish-promotions -once -category limpeza
go run ./cmd/publish-promotions -once -category alimentos
```

1. Faça login e deixe apenas **Alimentos** entre os interesses ativos.
2. Publique as duas ofertas acima. A lista deve receber somente a de Alimentos.
3. Cancele o interesse em Alimentos com a tela aberta e publique outra oferta da
   categoria. A oferta anterior permanece visível e a nova fica fora da lista.
4. Registre novamente o interesse e publique outra oferta para confirmar a
   entrega na mesma conexão.

Para testar a deduplicação no frontend, publique o mesmo evento duas vezes:

```powershell
go run ./cmd/publish-promotions -once -category alimentos -repeat 2
```

A publicação periódica pode ser habilitada pelo inicializador com
`-PublicarOfertas -Categoria alimentos -Intervalo 60s`.

## Verificação no RabbitMQ

Acesse [o painel do RabbitMQ](http://127.0.0.1:15672) e confira:

- Exchanges `eCommerce` (`direct`) e `Promocoes` (`topic`).
- Filas e bindings descritos na arquitetura.
- Consumidores ativos nas filas do fluxo web, incluindo uma `gateway.promocoes.*`.
- Mensagens prontas (Ready), em processamento (Unacked) e filas de revisão.
- Logs dos serviços e os registros persistidos, além do painel.

Fila vazia com consumidor ativo pode indicar processamento bem-sucedido.
Uma confirmação de publicação não demonstra que o consumidor concluiu o trabalho.
Não exclua filas, bindings ou mensagens para “limpar” a demonstração.

O painel precisa consultar o mesmo broker usado por `RABBITMQ_URL`.
Se o painel e o `rabbitmqctl` do Docker mostrarem filas diferentes, confira
se você está comparando o broker nativo com um broker Docker separado.

## Validação e arquivos locais

Com Gateway e Estoque em execução, teste a consulta GraphQL no Gateway:

```powershell
$body = @{
    query = '{ produtosDisponiveis(category: "eletronicos") { total products { id name quantity } } }'
} | ConvertTo-Json -Compress

Invoke-RestMethod 'http://localhost:8080/graphql' `
    -Method Post -ContentType 'application/json' -Body $body |
    ConvertTo-Json -Depth 6
```

Em uma resposta bem-sucedida, confira `data.produtosDisponiveis` e a ausência
de `errors`. Repita a mesma consulta em `http://localhost:8081/graphql` para
verificar o endpoint interno do Estoque. Para as operações autenticadas,
use o frontend e confira as requisições `POST /graphql` na aba Network.

Para verificar a sessão, cadastre uma conta, faça login e recarregue a página.
A Query `usuarioAtual` deve recuperar o usuário. Faça logout e recarregue
novamente. `usuarioAtual` deve indicar sessão ausente e o catálogo deve carregar.
A confirmação da sessão e a remoção do cookie podem ser conferidas nas respostas
de `entrar` e `sair`, pelo cabeçalho `Set-Cookie`.

Para testar a subscription de pedidos:

1. Faça login, abra **Meus pedidos** e procure `/graphql` em **Network → WS**. Confira o status `101 Switching Protocols`, a mensagem `connection_ack` e a operação `OrderUpdates`.
2. Crie um pedido e acompanhe o pagamento pelo Mock. Confira as mensagens `next` em `payload.data.pedidoAtualizado` e compare `orderId` e `status` com a tela.
3. Reinicie o Gateway com o frontend aberto. Confira uma nova conexão WebSocket e uma nova query `pedidos` após a reconexão.

```powershell
go build ./...
go vet ./...
Push-Location frontend
npm run build
Pop-Location
git diff --check
git diff --cached --check
```

Os testes Go ignorados pelo Git dependem dos arquivos presentes na máquina.
O teste versionado do inicializador usa dados simulados e pode ser executado sem
subir os serviços:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\test-start-dev.ps1
```

Na máquina que mantém os testes, use banco isolado `ecommerce_events_test`:

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
Remove-Item Env:TEST_RABBITMQ_URL -ErrorAction SilentlyContinue
go test ./... -count=1
```

Prepare esse banco separadamente com as migrations necessárias.
Não aponte testes de integração ao banco da demonstração. Não execute a suíte
inteira com `TEST_RABBITMQ_URL` apontando ao broker de negócio: alguns testes
de interesses publicam eventos reais. Testes de broker devem ser selecionados
e usar recursos isolados.

Simulação do inicializador sem abrir programas nem ler segredos:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -AbrirTerminais -WhatIf
```

## Observações

- Aplicação acadêmica local, não uma configuração pronta para produção.
- A lista de ofertas guarda até dez itens na sessão atual do frontend. A fila
  temporária do Gateway e a subscription entregam ofertas ao vivo.
- Cobranças do Mock ficam em memória: não reinicie durante um checkout.
- Nota fiscal, rastreio e despacho são simulados.
- O hub de notificações de pedidos fica em memória e atende uma instância do Gateway.
  A reconexão recupera o estado pela query `pedidos`.
- Alguns consumidores precisam de reinício após queda definitiva da conexão.
- A mutation `criarPedido` ainda pode criar outro pedido quando repetida.
- Não há garantia de entrega exatamente uma vez. Revisões precisam de diagnóstico,
  e retentativas devem preservar a identidade do evento.
- Segredos e chaves privadas são locais. Assinatura digital não substitui TLS
  ou controle de acesso ao broker.
- Os testes Go ignorados pelo Git ficam na máquina de desenvolvimento. O teste
  do inicializador em `scripts/test-start-dev.ps1` é versionado.

## Autores

- Gabriel de Almeida Spadafora
- Lucas de Lima Nogueira

## Licença

Projeto desenvolvido para fins acadêmicos.
