# RabbitMQ E-commerce

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

Promoções, Consumidor 1 e Consumidor 2 não participam do fluxo de assinatura digital e não precisam de chaves.

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

Os eventos da exchange `Promocoes` usam o mesmo envelope, mas deixam `signature` vazia e são consumidos sem validação criptográfica.

## Execução

Execute todos os comandos a partir da raiz do projeto. Use um terminal separado para cada processo.

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
- A URL do RabbitMQ está fixa para execução local e deve ser externalizada em uma configuração futura;
- O uso de JSON como persistência não substitui transações de banco de dados ou o padrão transactional outbox.

## Autores


- Gabriel de Almeida Spadafora

- Lucas de Lima Nogueira

## Licença

Projeto desenvolvido para fins acadêmicos.
