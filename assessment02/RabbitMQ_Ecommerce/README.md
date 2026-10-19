# Event Market - E-commerce com REST, RabbitMQ e SSE

Aplicação acadêmica de Sistemas Distribuídos da UTFPR, evolução da Avaliação 01.
Frontend em React/TypeScript; API Gateway e microsserviços em Go.
O fluxo atual persiste dados no PostgreSQL, usa RabbitMQ para eventos,
SSE para atualizações no navegador, Mock de Pagamento com webhook e Resend para promoções.

Autores: Gabriel de Almeida Spadafora e Lucas de Lima Nogueira.

## Funcionalidades

- Cadastro, login e consultas autenticadas dos próprios pedidos.
- Catálogo consultado pelo Gateway diretamente no MS Estoque via REST.
- Carrinho, criação de pedidos, reserva de produtos e consulta do histórico.
- Checkout em nova aba com aprovação ou recusa simulada.
- Expiração de cobranças, cancelamento e devolução de estoque.
- Nota fiscal e rastreamento simulados após pagamento aprovado.
- Notificações SSE de estoque confirmado, checkout, pagamento e envio.
- Preferências de promoções por categoria, inscrição com e-mail e cancelamento.
- Ofertas assinadas, processamento idempotente e envio pela API externa Resend.

Limitação mantida para a demonstração: o envio real de promoções é restrito
ao destinatário de teste configurado. Não está habilitado para qualquer e-mail.

## Arquitetura e responsabilidades

| Componente | Responsabilidade | Comunicação |
|---|---|---|
| Frontend | Catálogo, carrinho, pedidos, preferências e notificações | REST e SSE com Gateway |
| Gateway | Autenticação, pedidos, outboxes, inbox e SSE por cliente | REST com Estoque; eventos RabbitMQ |
| Estoque | Catálogo persistido, reserva e devolução | REST interno e RabbitMQ |
| Pagamento | Cobranças, webhook, decisão e expiração | REST com Mock e RabbitMQ |
| Mock de Pagamento | Checkout e escolha de aprovação/recusa | REST e webhook HTTP POST |
| Entrega | Nota fiscal e despacho simulados | RabbitMQ |
| Promoções | Interesses, ofertas e jobs de e-mail | RabbitMQ e API Resend |

PostgreSQL é uma única instância local com schemas de responsabilidade separados:
`gateway`, `estoque`, `pagamento`, `entrega` e `promocoes`.
Os serviços não precisam consultar diretamente as tabelas uns dos outros no fluxo normal.

Sequência de um pedido:

1. Frontend envia o pedido por REST; Gateway grava pedido e intenção de evento na mesma transação.
2. Publicador da outbox envia `pedido.criado`; Estoque verifica e reserva os itens.
3. Estoque publica `pedido.estoque_ok` ou `estoque.indisponivel`.
4. Pagamento solicita a cobrança ao Mock e publica a disponibilidade do checkout.
5. Usuário abre o checkout em nova aba; Mock envia a decisão ao webhook de Pagamento.
6. Pagamento publica aprovação ou recusa. Aprovação permite despacho pela Entrega.
7. Gateway consome os resultados, atualiza o banco e notifica o cliente por SSE.
8. Recusa ou expiração leva ao cancelamento e à compensação do estoque.

## RabbitMQ

| Exchange | Tipo | Uso |
|---|---|---|
| `eCommerce` | `direct` | Pedidos, resultados e interesses |
| `Promocoes` | `topic` | Ofertas por categoria |

| Fila | Routing keys |
|---|---|
| `estoque` | `pedido.criado`, `pedido.excluido` |
| `pagamento` | `pedido.estoque_ok` |
| `entrega` | `pagamento.aprovado` |
| `gateway.pedidos.status` | `pedido.estoque_ok`, `estoque.indisponivel`, `pagamento.checkout_disponivel`, `pagamento.aprovado`, `pagamento.recusado`, `pedido.enviado` |
| `promocoes.interesses` | `interesse.promocao` |
| `promocoes.ofertas` | `promocao.categoria.*` |

As categorias são alimentos, limpeza e eletronicos.
Filas legadas `principal` e `consumidor.01/02` não são necessárias para o fluxo web.
Elas podem acumular cópias se continuarem vinculadas sem consumidor: isso não
representa automaticamente falha das filas do fluxo web. Não as purgue sem avaliar os dados.

A topologia de pedidos é centralizada em `utils/rabbitmq/ecommerce_topology.go`.
O preparo idempotente declara filas duráveis e bindings sem apagá-los.
Atributos incompatíveis produzem erro; não se substituem filas automaticamente.

### Assinaturas e confiabilidade

Todas as mensagens de negócio RabbitMQ do fluxo atual usam RSA-PSS com SHA-256.
A assinatura cobre ID, tipo, produtor, instante e payload do envelope.
O consumidor verifica assinatura, produtor autorizado e compatibilidade com a routing key.
Assinatura não cifra o conteúdo: ela verifica autenticidade e integridade.

Publicações confirmadas usam mensagens persistentes, `mandatory` e publisher confirms.
Confirmação do broker não significa processamento pelo consumidor nem entrega de e-mail.
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

## Estrutura

- `cmd/`: pontos de entrada dos programas.
- `frontend/src/`: telas, cliente REST, SSE e estilos.
- `microservices/ms-gateway/`: autenticação, catálogo, pedidos e preferências.
- `microservices/ms-estoque/`, `ms-pagamento/`, `ms-entrega/`, `ms-promocoes/`: regras dos serviços.
- `microservices/mock-pagamento/`: provedor simulado.
- `utils/events/`, `utils/cryptography/`, `utils/rabbitmq/`, `utils/webhook/`: contratos, assinaturas e transporte.
- `migrations/`: alterações SQL numeradas 001 a 016.
- `scripts/start-dev.ps1`: inicializador do ambiente Windows.
- `data/`: arquivos de base/legados; as ofertas ainda usam `inventory.json`.

## Pré-requisitos e primeira preparação

Execute os comandos nesta pasta, que contém `go.mod` e `docker-compose.yml`.

- Go compatível com a versão declarada em `go.mod`.
- Node.js/npm compatíveis com o frontend e suas dependências.
- Docker Desktop aberto para executar PostgreSQL 17.
- RabbitMQ nativo do Windows instalado, serviço `RabbitMQ` e painel de gerenciamento habilitado.
- Windows PowerShell 5.1 ou superior.

Não execute os brokers nativo e Docker simultaneamente nas mesmas portas.
O inicializador foi feito para PostgreSQL no Docker e RabbitMQ nativo.
Não use `docker compose up -d` sem especificar o serviço nesse ambiente.

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
| `RESEND_FROM` | Remetente; no teste, `Event Market <onboarding@resend.dev>` |
| `RESEND_TEST_RECIPIENT` | E-mail associado à conta Resend dona da API key |
| `RESEND_API_KEY` | Chave privada de acesso à API externa, necessária somente para enviar |

Use segredos aleatórios próprios, não os exemplos públicos.
JWT, segredo de webhook, API key e chaves RSA têm funções diferentes.
Preserve o JWT para manter sessões após reinício; alterá-lo exige novo login.
Não publique `.env`, chaves privadas ou valores de segredos nos logs.

Segredos ausentes são pedidos com entrada oculta pelo inicializador.
Um segredo já configurado, mas curto, é rejeitado: ajuste o ambiente ou `.env`.
Comentários ao final de valores e interpolação de variáveis não são suportados pelo leitor de `.env`.

### Chaves RSA

Em instalação nova, sem a pasta `keys`:

```powershell
go run ./cmd/keygen
go run ./cmd/keygen -promotions-only
```

O primeiro gera pares de Principal, Estoque, Pagamento e Entrega e distribui chaves públicas.
O segundo acrescenta o par de Promoções. Não regenere nem apague chaves já utilizadas
apenas para tentar resolver um erro; eventos pendentes podem depender delas.
O Gateway usa a identidade criptográfica de Principal no fluxo web.

### PostgreSQL e migrations

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

## Inicialização no Windows

Feche os programas anteriores com Ctrl+C antes de iniciar.
O script não instala dependências, gera chaves, aplica migrations ou encerra programas à força.

```powershell
# Conferência sem iniciar serviços nem solicitar segredos.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -Verificar

# Fluxo web; sem envio real de e-mails e sem gerador automático.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -AbrirTerminais

# Autoriza também e-mails REAIS, inclusive jobs antigos pendentes.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -AbrirTerminais -ComEmails

# Inclui ofertas periódicas.
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\start-dev.ps1 -AbrirTerminais -ComEmails -PublicarOfertas -Categoria alimentos -Intervalo 60s
```

Oito terminais são abertos: Estoque, Mock, Pagamento, Entrega, interesses,
ofertas, Gateway e frontend. O script aguarda saúde HTTP e consumidores nas
seis filas. `-ComEmails` e `-PublicarOfertas` são opcionais para evitar disparos involuntários.

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
| Catálogo interno do Estoque | http://localhost:8081/internal/products |
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
go run ./cmd/promocoes -offers
go run ./cmd/gateway
# Em outro terminal, dentro de frontend:
npm run dev
# Opcional, em terminal separado:
go run ./cmd/promocoes -emails
```

Cada comando acima é um processo separado. Principal e consumidores legados
1/2 não são necessários; o menu legado de pedidos usa contratos antigos.

## API REST e SSE

| Método | Rota | Uso |
|---|---|---|
| POST | `/api/auth/register` | Cadastro |
| POST | `/api/auth/login` | Login |
| POST | `/api/auth/logout` | Logout |
| GET | `/api/auth/me` | Sessão atual |
| GET | `/api/products` | Catálogo consultado no Estoque |
| POST | `/api/orders` | Criar pedido |
| GET | `/api/orders` | Listar pedidos do cliente |
| GET | `/api/orders/{orderId}` | Detalhes |
| DELETE | `/api/orders/{orderId}` | Cancelamento permitido pela regra atual |
| GET | `/api/orders/events` | Stream SSE autenticado |
| GET | `/api/promotion-interests` | Preferências |
| POST | `/api/promotion-interests` | Inscrição com categoria/e-mail |
| DELETE | `/api/promotion-interests/{category}` | Cancelar interesse |

IDs internos de pedidos são UUIDs; identificadores de exibição `PED-...`
são usados nos contratos com os serviços.
Pedidos, detalhes e SSE são vinculados ao usuário autenticado.

`201` na criação significa que o pedido e a intenção do evento foram persistidos,
não que estoque ou pagamento já foram concluídos. Repetir um POST pode criar
outro pedido: idempotência do evento não é idempotência da requisição HTTP.

O cancelamento manual só é permitido em `PENDENTE`, antes de preparar o envelope
de criação. A transação retira essa intenção ainda não enviada e grava a compensação.
Depois desse ponto, a API retorna 409 para não liberar estoque enquanto uma
cobrança pode ser aprovada. Não há exclusão física de pedido, itens ou histórico.

Pagamento é a única autoridade de expiração. A cobrança vencida gera
`pagamento.recusado` com motivo `CHECKOUT_EXPIRED`; Gateway cancela e publica
`pedido.excluido` para compensar. O antigo worker de expiração do Gateway não
é iniciado em `cmd/gateway`. Aprovação e expiração disputam a mesma cobrança
sob controle transacional, evitando decisões locais independentes.

SSE envia mudanças após commit, apenas às conexões do cliente.
Os avisos ficam empilhados no canto e cada um desaparece após oito segundos.
Reconexão sincroniza o estado atual por REST, mas não reproduz obrigatoriamente
todos os avisos transitórios perdidos enquanto o navegador esteve desconectado.
A atualização automática do catálogo é separada do stream de pedidos.

## Promoções e modo de teste

Inscreva o e-mail permitido e uma categoria na interface. Interesses passam pela
outbox do Gateway e são consumidos pelo MS Promoções.
O consumidor de ofertas verifica assinatura/categoria e cria jobs idempotentes.
O worker confere interesse ativo antes de enviar e registra resultado/tentativas.
Cancelar interesse desativa a inscrição e cancela os jobs pendentes correspondentes.

Publicação manual, com os consumidores já iniciados:

```powershell
go run ./cmd/publish-promotions -once -category alimentos
# Mesmo evento duas vezes para demonstrar deduplicação:
go run ./cmd/publish-promotions -once -category alimentos -repeat 2
```

O gerador usa `data/inventory.json`, não o catálogo REST real.
Os dois modos de publicação de ofertas usam assinatura e confirmação do RabbitMQ.
O modo legado `go run ./cmd/promocoes` sem flags também gera ofertas e pode
alimentar o fluxo de e-mails; não o abra por engano em paralelo ao gerador escolhido.

Uma API key identifica a aplicação/conta Resend, não cada destinatário.
Com `onboarding@resend.dev`, o teste se limita ao e-mail associado à conta dona
da chave. Ter outra conta Resend não libera esse outro e-mail na mesma chave.
O próprio código também restringe o destinatário a `RESEND_TEST_RECIPIENT`.

Para vários destinatários reais seria necessário verificar um domínio remetente,
ajustar `RESEND_FROM` e adaptar a restrição de teste do código. Isso não foi habilitado.
Regras do provedor: https://resend.com/docs/knowledge-base/403-error-resend-dev-domain

`SENT` e um ID do provedor significam aceitação pela API, não prova de chegada
à caixa de entrada. Verifique também a caixa de e-mail e spam na demonstração.
Não zere tentativas nem recrie jobs antigos indiscriminadamente: a janela de
idempotência do provedor é limitada e isso pode duplicar e-mails.

## Validação e arquivos locais

```powershell
go build ./...
go vet ./...
Push-Location frontend
npm run build
Pop-Location
git diff --check
git diff --cached --check
```

Por decisão da equipe, arquivos com `_test` no nome e READMEs auxiliares ficam
somente na máquina e são ignorados pelo Git. Apenas este README é central.
Um clone novo não recebe esses testes: `go test ./...` sem arquivos de teste
não comprova a mesma cobertura da máquina de desenvolvimento.
A remoção de rastreamento não apaga arquivos locais nem o histórico Git anterior.

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

## Licença

Projeto desenvolvido para fins acadêmicos.
