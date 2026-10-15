# E-commerce Web - REST, RabbitMQ e SSE

Evolução do projeto da Avaliação 01 para uma aplicação web com frontend React,
API Gateway em Go, PostgreSQL, RabbitMQ, SSE, pagamento via webhook e envio de
promoções por e-mail.

## Estado atual

- código da Avaliação 01 preservado como base;
- configuração centralizada por variáveis de ambiente;
- PostgreSQL e RabbitMQ definidos no Docker Compose;
- schema inicial do banco criado em `migrations/001_initial.sql`;
- API Gateway HTTP inicial com `GET /` e `GET /healthz`;
- frontend React + TypeScript + Vite inicial;
- Gateway conectado ao PostgreSQL e autenticação com telas de acesso;
- catálogo REST e interface validados com Estoque simulado;
- carrinho e criação, listagem e detalhes de pedidos persistidos no PostgreSQL;
- publicação assinada de pedido.criado validada em fila de teste no RabbitMQ;
- falha de publicação registrada no estado e histórico do pedido;
- consumo assinado, inbox e atualização de status integrados pelo PR #19;
- cancelamento autenticado com link condicional e confirmação na interface,
  integrado pelo PR #20;
- outbox de compensação com assinatura preservada, confirmação do RabbitMQ,
  recuperação após indisponibilidade e limite de tentativas validados;
- MS Estoque com PostgreSQL, endpoint interno de catálogo e reserva por
  eventos assinados (Etapa 13);
- MS Pagamento com cobrança no Mock de Pagamento, checkout em nova aba e
  webhook assinado com HMAC, validados com o Gateway (Etapa 14);
- integração com Entrega real pendente; testes atuais usam mocks e filas de
  captura (ver divisão de trabalho atualizada abaixo);
- SSE autenticado para status e checkout, atualização automática da interface,
  notificações e reconexão integrados pelo PR #21;
- link condicional de checkout, abertura em nova aba e indicação de expiração
  validados com evento assinado simulado e integrados pelo PR #22;
- cancelamento automático por expiração implementado na branch
  `feat/checkout-expiration`, com compensação pela outbox e atualização por SSE;
  demonstração local validada, integração em `main` pendente (Etapa 15).

## Atualização da divisão de trabalho - Promoções

Em 12/10/2026, Lucas decidiu assumir o fluxo completo de promoções na branch
`feat/promotion-preferences`, incluindo a parte inicialmente atribuída a Gabriel:

- tela de preferências e endpoints autenticados no Gateway;
- persistência da escolha do cliente e publicação confiável de `interesse.promocao`;
- consumo idempotente dos interesses no MS Promoções e cancelamento da inscrição;
- geração/publicação das ofertas por categoria usando as routing keys existentes;
- integração com API externa de e-mail e registro das tentativas de envio;
- testes e documentação do fluxo.

O fluxo de promoções foi integrado pelo PR #23 e demonstrado: preferências
autenticadas, outbox do Gateway, consumo dos interesses, ofertas assinadas pelo
RabbitMQ, geração manual/aleatória e envio de e-mail pelo Resend. Foram validados
o recebimento real, o cancelamento do interesse e duas entregas do mesmo evento
criando somente um envio por usuário.

O envio permanece restrito a `RESEND_TEST_RECIPIENT`. O gerador usa o catálogo
local `data/inventory.json`; não consulta o serviço real de Estoque. O modo
automático publica imediatamente e depois a cada 60 segundos, por padrão.

Estoque, Pagamento e Mock foram implementados por Gabriel e já estão na `main`.
A proposta anterior de Lucas assumir Pagamento/Mock não gerou outra implementação:
o trabalho local foi retirado para evitar duplicação. Lucas implementa a expiração
no Gateway; Entrega permanece como próxima integração de Gabriel. A manutenção e
a apresentação de Pagamento/Mock devem ser combinadas entre os dois.
Detalhes em [Divisão de responsabilidades](docs/steps/00-divisao-equipe/README.md).

## Guias por etapa

- [Etapa 00 - Divisão de responsabilidades](docs/steps/00-divisao-equipe/README.md)
- [Etapa 01 - Infraestrutura local](docs/steps/01-infraestrutura/README.md)
- [Etapa 02 - Contratos de integração](docs/steps/02-contratos/README.md)
- [Etapa 03 - Integração entre Gateway e PostgreSQL](docs/steps/03-gateway-postgres/README.md)
- [Etapa 04 - Autenticação e telas de acesso](docs/steps/04-autenticacao/README.md)
- [Etapa 05 - Catálogo no Gateway e React](docs/steps/05-catalogo/README.md)
- [Etapa 06 - Carrinho e pedidos (em andamento)](docs/steps/06-carrinho-pedidos/README.md)
- [Etapa 07 - Eventos de pedidos (em andamento)](docs/steps/07-eventos-pedidos/README.md)
- [Etapa 08 - Consumo de eventos e status dos pedidos (em andamento)](docs/steps/08-status-pedidos/README.md)
- [Etapa 09 - Cancelamento e compensação (em andamento)](docs/steps/09-cancelamento-compensacao/README.md)
- [Etapa 10 - Atualização de pedidos por SSE (em andamento)](docs/steps/10-sse-pedidos/README.md)
- [Etapa 11 - Link de checkout dos pedidos](docs/steps/11-checkout-pedidos/README.md)
- [Etapa 12 - Preferências, ofertas e e-mail de promoções](docs/steps/12-promocoes-email/README.md)
- [Etapa 13 - MS Estoque com PostgreSQL](docs/steps/13-estoque-postgres/README.md)
- [Etapa 14 - MS Pagamento e Mock de Pagamento](docs/steps/14-pagamento-webhook/README.md)
- [Etapa 15 - Expiração automática do checkout](docs/steps/15-expiracao-checkout/README.md)

## Configuração inicial

1. Copie `.env.example` para `.env`.
2. Configure `JWT_SECRET` no ambiente do terminal com pelo menos 32 bytes.
   O Gateway não carrega `.env` automaticamente. Preserve o mesmo segredo ao
   reiniciar para manter as sessões existentes válidas.
3. Inicie PostgreSQL e RabbitMQ com `docker compose up -d`.
4. Execute o Gateway com `go run ./cmd/gateway`.
5. Em outro terminal, entre em `frontend`, execute `npm install` e depois
   `npm run dev`.
6. Abra `http://localhost:5173`.

O endpoint `http://localhost:8080/healthz` deve responder com o estado saudável
do API Gateway. O painel do RabbitMQ fica disponível em
`http://localhost:15672`.

## Projeto original

Sistema distribuído de E-Commerce orientado a eventos, desenvolvido em Go e RabbitMQ como atividade acadêmica da disciplina de Sistemas Distribuídos.

O projeto simula o processamento assíncrono de pedidos entre os microsserviços Principal, Estoque, Pagamento e Entrega, além da publicação e do consumo de promoções por categoria. Os eventos relacionados a pedidos utilizam assinatura digital RSA-PSS com SHA-256 para validar sua autenticidade e integridade.

## Funcionalidades

- Cadastro e consulta de pedidos pelo terminal;
- Consulta do catálogo de produtos e estoque;
- Reserva e devolução de estoque;
- Aprovação ou recusa simulada de pagamentos;

- Publicação periódica de promoções por categoria;
- Consumidores de promoções com bindings diferentes;
- Persistência local de pedidos e estoque em JSON;
- Assinatura digital dos eventos do fluxo de pedidos;
- Descarte de eventos com assinatura inválida.

## Arquitetura

<p align="center">
  <img
    src="docs/architecture.png"
    alt="Arquitetura do sistema RabbitMQ E-commerce"
    width="900"
  >
</p>

As mensagens não são publicadas diretamente nas filas. Cada produtor envia o evento para uma exchange utilizando uma routing key. O RabbitMQ encaminha a mensagem às filas que possuem bindings compatíveis.

### Exchanges

| Exchange | Tipo | Finalidade |
|---|---|---|
| `eCommerce` | `direct` | Processamento de pedidos, estoque, pagamento e entrega |
| `Promocoes` | `topic` | Distribuição de promoções por categoria |

### Filas

| Fila | Consumidor | Eventos principais |
|---|---|---|
| `principal` | Principal | Respostas de estoque, pagamento e entrega |
| `estoque` | Estoque | `pedido.criado` e `pedido.excluido` |
| `pagamento` | Pagamento | `pedido.estoque_ok` |
| `entrega` | Entrega | `pagamento.aprovado` |
| `consumidor.01` | Consumidor 1 | Promoções de Limpeza e Alimentos |
| `consumidor.02` | Consumidor 2 | Promoções de todas as categorias |

### Eventos do E-Commerce

| Routing key | Produtor | Consumidores |
|---|---|---|
| `pedido.criado` | Principal | Estoque |
| `pedido.excluido` | Principal | Estoque |
| `pedido.estoque_ok` | Estoque | Principal e Pagamento |
| `estoque.indisponivel` | Estoque | Principal |
| `pagamento.aprovado` | Pagamento | Principal e Entrega |
| `pagamento.recusado` | Pagamento | Principal |
| `pedido.enviado` | Entrega | Principal |

### Eventos de promoção

| Routing key | Categoria |
|---|---|
| `promocao.categoria.limpeza` | Limpeza |
| `promocao.categoria.alimentos` | Alimentos |
| `promocao.categoria.eletronicos` | Eletrônicos |
| `promocao.categoria.*` | Todas as categorias |

## Estrutura do projeto

```text
RabbitMQ_Ecommerce/
├── cmd/
│   ├── consumidores/
│   ├── entrega/
│   ├── estoque/
│   ├── keygen/
│   ├── pagamento/
│   ├── principal/
│   ├── promocoes/
├── data/
│   ├── inventory.json
│   └── orders.json
├── keys/
│   ├── ms-entrega/
│   ├── ms-estoque/
│   ├── ms-pagamento/
│   └── ms-principal/
├── microservices/
│   ├── ms-consumidores/
│   ├── ms-entrega/
│   ├── ms-estoque/
│   ├── ms-pagamento/
│   ├── ms-principal/
│   └── ms-promocoes/
├── utils/
│   ├── cryptography/
│   ├── events/
│   ├── inventory/
│   ├── misc/
│   └── rabbitmq/
├── go.mod
└── go.sum
```

Os diretórios dentro de `cmd` contêm os pontos de entrada dos processos. A lógica de cada microsserviço fica em `microservices`, enquanto os contratos de eventos e recursos compartilhados ficam em `utils`.

## Tecnologias

- Go 1.27.1;
- RabbitMQ;
- Protocolo AMQP 0-9-1;
- Biblioteca `github.com/rabbitmq/amqp091-go`;
- RSA-PSS e SHA-256 da biblioteca padrão do Go;
- Arquivos JSON para persistência local.

## Pré-requisitos

- Go instalado e disponível no `PATH`;
- RabbitMQ em execução;
- Plugin de gerenciamento do RabbitMQ habilitado, caso a interface web seja utilizada;
- Portas `5672` e `15672` disponíveis.

Por padrão, a aplicação utiliza:

```text
AMQP:       amqp://guest:guest@localhost:5672/
Management: http://localhost:15672
```

As credenciais padrão devem ser utilizadas somente em ambiente local.

## Preparação

Clone o repositório e entre no diretório do projeto:

```powershell
git clone <URL_DO_REPOSITORIO>
cd RabbitMQ_Ecommerce
```

Baixe e valide as dependências:

```powershell
go mod download
go mod verify
```

Formate e compile todos os pacotes:

```powershell
gofmt -w .
go test ./...
```

## Geração e distribuição das chaves

Execute o gerador uma única vez, a partir da raiz do projeto:

```powershell
go run ./cmd/keygen
```

O comando gera os pares RSA de Principal, Estoque, Pagamento e Entrega e distribui as chaves públicas entre esses microsserviços. Ele recusa sobrescrever uma pasta `keys` existente.

Cada serviço mantém sua chave privada localmente e recebe cópias das chaves públicas dos demais:

```text
keys/ms-principal/
├── private.pem
├── public.pem
└── public_keys/
    ├── ms-entrega.pem
    ├── ms-estoque.pem
    └── ms-pagamento.pem
```

Nunca envie ou versione os arquivos `private.pem`:

```gitignore
**/private.pem
*.exe
```

O novo publicador de Promoções também assina seus eventos. Para adicionar suas
chaves sem substituir as existentes, execute `go run ./cmd/keygen -promotions-only`
uma única vez. Os consumidores legados 1 e 2 não precisam de chave privada.

## Assinatura digital

Os eventos enviados pela exchange `eCommerce` seguem este processo:

1. O produtor serializa os campos assináveis do envelope;
2. Calcula o hash SHA-256;
3. Assina o hash com RSA-PSS e sua chave privada;
4. Codifica a assinatura em Base64 e preenche `signature`;
5. O consumidor identifica o produtor e carrega sua chave pública;
6. Recalcula o hash e verifica a assinatura antes de executar o handler.

São assinados os campos:

- `event_id`;
- `event_type`;
- `producer`;
- `timestamp`;
- `payload`.

O próprio campo `signature` não participa do cálculo. Eventos sem assinatura, assinados por uma chave incorreta ou alterados depois da assinatura são descartados sem reenvio.

No fluxo web desta branch, `cmd/publish-promotions` assina as ofertas e o modo
`cmd/promocoes -offers` valida a assinatura e a categoria antes de criar os
envios. O publicador legado `cmd/promocoes` sem flags continua sem assinatura;
suas mensagens serão rejeitadas pelo novo consumidor. Não usá-lo na demonstração
do envio de e-mails.

## Execução

Execute todos os comandos a partir da raiz do projeto. Use um terminal separado para cada processo.

No fluxo web, `cmd/estoque` e `cmd/pagamento` já seguem os contratos da
Avaliação 02 e precisam do PostgreSQL; o Pagamento também precisa de
`PAYMENT_WEBHOOK_SECRET` e de `cmd/mock-pagamento`. Veja as
[Etapas 13](docs/steps/13-estoque-postgres/README.md) e
[14](docs/steps/14-pagamento-webhook/README.md). O menu do `cmd/principal`
abaixo publica o formato antigo de pedido e não é mais aceito pelo Estoque.

Inicie os serviços consumidores antes de produzir pedidos:

```powershell
go run ./cmd/estoque
```

```powershell
go run ./cmd/pagamento
```

```powershell
go run ./cmd/entrega
```

Depois inicie o Principal:

```powershell
go run ./cmd/principal
```

O Principal disponibiliza um menu para consultar produtos, criar pedidos, excluir pedidos e consultar seus estados.

### Promoções

Para o fluxo web com Resend, siga a [Etapa 12](docs/steps/12-promocoes-email/README.md).
Ela documenta os processos separados `-interests`, `-offers`, `-emails` e o
publicador manual/automático. Os comandos abaixo são da simulação legada.

Inicie primeiro os consumidores:

```powershell
go run ./cmd/consumidores 1
```

```powershell
go run ./cmd/consumidores 2
```

Depois inicie o produtor:

```powershell
go run ./cmd/promocoes
```

O Consumidor 1 recebe promoções de Limpeza e Alimentos. O Consumidor 2 utiliza o padrão `promocao.categoria.*` e recebe promoções de todas as categorias.

## Persistência local

O projeto utiliza:

- `data/inventory.json`: catálogo e quantidades em estoque;
- `data/orders.json`: pedidos, próximo identificador e estados de processamento.

O Estoque é responsável por persistir baixas e devoluções no inventário. O Principal persiste os pedidos e atualiza seus estados conforme os eventos recebidos.

Essa persistência atende à simulação acadêmica local. Em uma implantação distribuída real, os microsserviços não deveriam compartilhar arquivos; cada serviço manteria seu próprio banco de dados ou armazenamento.



## Verificação no RabbitMQ

Acesse [http://localhost:15672](http://localhost:15672) e confira:

- as exchanges `eCommerce` e `Promocoes`;
- as filas declaradas;
- os bindings e suas routing keys;
- a quantidade de mensagens prontas e não confirmadas.

## Observações

- Exchanges e filas duráveis não garantem sozinhas entrega exatamente uma vez;
- Consumidores usam confirmação manual (`Ack`/`Nack`);
- Eventos inválidos não são reenfileirados;
- Os handlers devem ser idempotentes para lidar com possíveis duplicações;
- A URL do RabbitMQ é configurada por `RABBITMQ_URL`; usar credenciais locais somente na demonstração;
- O uso de JSON como persistência não substitui transações de banco de dados ou o padrão transactional outbox.

## Autores


- Gabriel de Almeida Spadafora

- Lucas de Lima Nogueira

## Licença

Projeto desenvolvido para fins acadêmicos.
