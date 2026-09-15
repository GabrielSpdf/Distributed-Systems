# Distributed Systems

Repositório destinado aos trabalhos e projetos desenvolvidos durante a disciplina de Sistemas Distribuídos.

O conteúdo será atualizado ao longo do semestre conforme novas avaliações forem desenvolvidas.

## Projetos

| Avaliação | Projeto | Tecnologias | Situação |
|---|---|---|---|
| 01 | [RabbitMQ E-commerce](assessment01/RabbitMQ_Ecommerce) | Go, RabbitMQ, AMQP, RSA-PSS e JSON | Concluído |

## Avaliação 01 — RabbitMQ E-commerce

Sistema distribuído de E-Commerce orientado a eventos, desenvolvido em Go e RabbitMQ.

O projeto simula a comunicação assíncrona entre microsserviços responsáveis por:

- Gerenciamento de pedidos;
- Controle de estoque;
- Processamento de pagamentos;
- Envio de pedidos;
- Publicação e consumo de promoções.

A comunicação utiliza exchanges `direct` e `topic`, filas e routing keys. Os eventos relacionados ao processamento de pedidos possuem assinatura digital RSA-PSS com SHA-256.

Para consultar a documentação completa, as instruções de execução e a arquitetura do projeto, acesse:

[Documentação do RabbitMQ E-commerce](assessment01/RabbitMQ_Ecommerce/README.md)

## Estrutura do repositório

```text
Distributed-Systems/
├── assessment01/
│   └── RabbitMQ_Ecommerce/
│       └── README.md
└── README.md