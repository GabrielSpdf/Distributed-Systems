# Distributed Systems

Repositório dos trabalhos desenvolvidos na disciplina de Sistemas Distribuídos
da Universidade Tecnológica Federal do Paraná (UTFPR).

Os projetos mostram a evolução de um e-commerce orientado a eventos:
da aplicação em terminal com RabbitMQ para uma aplicação web com REST,
SSE, persistência transacional e integrações externas.

## Projetos

| Trabalho | Projeto | Tecnologias principais | Documentação |
|---|---|---|---|
| 01 | RabbitMQ E-commerce | Go, RabbitMQ, AMQP, RSA-PSS e JSON | [README do Trabalho 01](assessment01/RabbitMQ_Ecommerce/README.md) |
| 02 | Event Market - E-commerce Web | React, TypeScript, Go, REST, SSE, RabbitMQ, PostgreSQL e Resend | [README do Trabalho 02](assessment02/RabbitMQ_Ecommerce/README.md) |

## Trabalho 01 - RabbitMQ E-commerce

Simulação de processamento assíncrono de pedidos pelo terminal, com microsserviços
de Principal, Estoque, Pagamento e Entrega, além de promoções por categoria.

- Comunicação por exchanges `direct` e `topic`, filas e routing keys.
- Reserva e devolução de estoque, pagamento e envio simulados.
- Persistência local em arquivos JSON.
- Assinatura digital RSA-PSS com SHA-256 no fluxo de pedidos.

[Arquitetura, configuração e execução do Trabalho 01](assessment01/RabbitMQ_Ecommerce/README.md).

## Trabalho 02 - Event Market

Evolução para uma aplicação web, com frontend em linguagem diferente do backend,
API Gateway, atualização de pedidos em tempo real e integrações externas.

- Frontend React/TypeScript com catálogo, carrinho, pedidos e preferências.
- API Gateway em Go com autenticação, REST e SSE por cliente.
- Consulta direta ao MS Estoque via REST.
- PostgreSQL para pedidos, reservas, cobranças, entregas e promoções.
- Mock de Pagamento em nova aba, com retorno por webhook.
- Nota fiscal e rastreamento simulados.
- Promoções com API externa Resend, em modo de destinatário de teste.
- Eventos RabbitMQ assinados, publicação confirmada, outbox e idempotência.
- Inicializador PowerShell para o ambiente local Windows.

[Arquitetura, configuração e execução do Trabalho 02](assessment02/RabbitMQ_Ecommerce/README.md).

O Trabalho 02 não substitui os arquivos do Trabalho 01. Cada pasta mantém
seus próprios comandos, contratos e instruções de execução.

## Evolução entre os trabalhos

| Aspecto | Trabalho 01 | Trabalho 02 |
|---|---|---|
| Interface | Terminal | Aplicação web React/TypeScript |
| Entrada de pedidos | Principal com menu | API Gateway REST |
| Atualizações ao usuário | Terminal | SSE autenticado e interface atualizada |
| Persistência do fluxo principal | JSON local | PostgreSQL |
| Pagamento | Simulação no serviço | Mock externo com checkout e webhook |
| Promoções | Consumidores por categoria | Preferências e envio pela API Resend |
| Assinaturas | Fluxo de pedidos | Pedidos, interesses e ofertas do fluxo atual |
| Recuperação e repetição | Simulação inicial | Outboxes, inbox, confirmações e idempotência |

## Estrutura do repositório

```text
Distributed-Systems/
├── assessment01/
│   └── RabbitMQ_Ecommerce/
│       ├── cmd/
│       ├── data/
│       ├── docs/
│       ├── microservices/
│       ├── utils/
│       ├── go.mod
│       ├── go.sum
│       └── README.md
├── assessment02/
│   └── RabbitMQ_Ecommerce/
│       ├── cmd/
│       ├── frontend/
│       ├── microservices/
│       ├── migrations/
│       ├── scripts/
│       ├── utils/
│       ├── docker-compose.yml
│       └── README.md
└── README.md
```

## Como executar

Escolha o trabalho e siga seu README. Execute os comandos na pasta do projeto
correspondente, não na raiz deste repositório.

No Trabalho 02, o inicializador local usa PostgreSQL no Docker e RabbitMQ
nativo do Windows. Não abra simultaneamente os brokers nativo e Docker nas
mesmas portas. Dependências, migrations, chaves e segredos devem ser preparados
conforme as instruções do projeto.

## Autores

- Gabriel de Almeida Spadafora
- Lucas de Lima Nogueira

## Licença

Projetos desenvolvidos para fins acadêmicos.
