# Etapa 00 - Divisão de responsabilidades

Este documento organiza o trabalho da dupla. A divisão busca equilibrar o peso
das tarefas e reduzir conflitos nos mesmos arquivos. Apesar da separação, os
dois integrantes devem entender o fluxo completo para a defesa da aplicação.

## Visão geral

| Integrante | Área principal | Resultado esperado |
|---|---|---|
| Lucas | Experiência do cliente e orquestração | React, autenticação, pedidos, API Gateway e SSE |
| Gabriel | Processamento e integrações | Estoque, pagamento, entrega, promoções e serviços externos |
| Ambos | Contratos e qualidade | Eventos, segurança, infraestrutura, testes, documentação e defesa |

---

## Lucas - Frontend e API Gateway

### Responsabilidades

1. Frontend React e TypeScript:
   - cadastro;
   - login;
   - catálogo;
   - carrinho;
   - criação de pedidos;
   - tela de acompanhamento;
   - preferências de promoções;
   - conexão SSE.

2. API Gateway/MS Principal:
   - endpoints REST públicos;
   - definição e manutenção do contrato OpenAPI público;
   - validação das requisições;
   - cadastro e autenticação;
   - emissão e validação de JWT;
   - persistência de usuários e pedidos;
   - publicação de `pedido.criado` e `pedido.excluido`;
   - consumo dos eventos de Estoque, Pagamento e Entrega;
   - gerenciamento das conexões SSE;
   - isolamento dos pedidos por cliente.

3. Persistência sob responsabilidade do Gateway:
   - `gateway.users`;
   - `gateway.orders`;
   - `gateway.order_items`;
   - `gateway.order_status_history`.

### Diretórios principais

```text
frontend/
cmd/gateway/
microservices/ms-gateway/
```

### Entregáveis

- usuário consegue criar conta e entrar;
- catálogo é exibido no React;
- pedido é enviado pelo Gateway;
- cliente consulta apenas os próprios pedidos;
- mudanças de estado aparecem em tempo real por SSE.

---

## Gabriel - Microsserviços e integrações externas

### Responsabilidades

1. MS Estoque:
   - persistência dos produtos;
   - endpoint REST interno para consulta do catálogo;
   - consumo de `pedido.criado` e `pedido.excluido`;
   - reserva e devolução transacional do estoque;
   - publicação de `pedido.estoque_ok` ou `estoque.indisponivel`.

2. MS Pagamento:
   - consumo de `pedido.estoque_ok`;
   - solicitação de cobrança ao Mock;
   - persistência das cobranças;
   - endpoint de webhook;
   - idempotência dos webhooks;
   - publicação de `pagamento.aprovado` ou `pagamento.recusado`.

3. Mock de Pagamento:
   - criação da cobrança por REST;
   - página de checkout;
   - botões para aprovação e recusa;
   - chamada do webhook do MS Pagamento.

4. MS Entrega:
   - consumo de `pagamento.aprovado`;
   - geração simulada de nota fiscal e rastreamento;
   - publicação de `pedido.enviado`.

5. MS Promoções:
   - consumo de `interesse.promocao`;
   - persistência das categorias escolhidas;
   - cancelamento do interesse;
   - integração com a API externa de e-mail;
   - registro de sucesso ou falha do envio.

6. Infraestrutura dos serviços:
   - topologia de exchanges, filas, bindings e routing keys;
   - manutenção do Docker Compose, com revisão de Lucas;
   - migrations dos schemas `estoque`, `pagamento` e `promocoes`.

### Diretórios principais

```text
cmd/estoque/
cmd/pagamento/
cmd/entrega/
cmd/promocoes/
cmd/mock-pagamento/
microservices/ms-estoque/
microservices/ms-pagamento/
microservices/ms-entrega/
microservices/ms-promocoes/
```

### Entregáveis

- catálogo vem do banco do Estoque;
- reserva e compensação funcionam;
- checkout abre no navegador;
- webhook determina o resultado do pagamento;
- pedido aprovado chega ao serviço de Entrega;
- cliente interessado recebe o e-mail de promoção.

---

## Responsabilidades compartilhadas

As tarefas abaixo devem ser revisadas pelos dois integrantes:

- contratos JSON dos eventos;
- nomes de exchanges, filas e routing keys;
- assinatura RSA-PSS e validação dos produtores;
- configuração do RabbitMQ;
- migrations e convenções do PostgreSQL;
- Docker Compose;
- tratamento de erros e logs;
- testes de integração;
- documentação;
- diagrama de arquitetura;
- roteiro e ensaio da defesa.

Arquivos compartilhados exigem coordenação antes de serem alterados:

```text
utils/events/
utils/rabbitmq/
utils/cryptography/
docker-compose.yml
migrations/
README.md
```

---

## Equilíbrio e independência

A divisão é considerada equilibrada pela combinação de profundidade e
quantidade de componentes:

- Lucas possui menos serviços, mas concentra as áreas de maior profundidade:
  frontend, autenticação, autorização por cliente, orquestração e SSE;
- Gabriel possui mais serviços, porém menores e bem delimitados, além da
  infraestrutura RabbitMQ e das integrações externas;
- contratos de eventos, segurança, testes integrados e defesa permanecem sob
  responsabilidade dos dois.

A separação de tarefas não garante independência por si só. Para que ninguém
fique bloqueado aguardando o outro, os contratos abaixo devem ser definidos e
aprovados antes da implementação das funcionalidades.

### Pontos de acoplamento conhecidos

- o Gateway depende do endpoint de produtos do Estoque;
- o Estoque depende do payload de `pedido.criado`;
- o acompanhamento SSE depende dos eventos dos microsserviços;
- o MS Pagamento depende da API e do webhook do Mock;
- `utils/events`, `docker-compose.yml` e migrations são áreas compartilhadas;
- o fluxo completo depende da preservação do mesmo `order_id` entre serviços.

### Contrato 1 - Gateway para Estoque

Endpoint interno:

```text
GET /internal/products
```

Resposta:

```json
[
  {
    "id": "PROD-001",
    "name": "Arroz 5 kg",
    "category": "alimentos",
    "price": 29.90,
    "quantity": 30
  }
]
```

Lucas poderá utilizar uma resposta simulada enquanto Gabriel implementa o
endpoint real.

### Contrato 2 - Envelope RabbitMQ

```json
{
  "event_id": "UUID",
  "event_type": "pedido.criado",
  "producer": "ms-principal",
  "timestamp": "2026-09-30T18:00:00Z",
  "payload": {},
  "signature": "..."
}
```

Antes da implementação, a dupla deve definir para cada evento:

- routing key;
- produtor autorizado;
- consumidores;
- campos do payload;
- transição de estado resultante.

### Contrato 3 - Gateway para React por SSE

```text
event: order.status.changed
id: EVENT_ID
data: {
  "orderId": "PED-001",
  "status": "PAGAMENTO_APROVADO",
  "message": "Pagamento aprovado"
}
```

Esse formato permite desenvolver a tela de acompanhamento usando eventos
simulados antes da integração com o RabbitMQ.

### Contrato 4 - MS Pagamento e Mock

```text
POST /charges
POST /webhooks/payments
```

O contrato deve definir os identificadores da cobrança e do pedido, o valor, a
URL de checkout, os estados possíveis e o identificador único do webhook para
garantir idempotência.

### Contrato 5 - Propriedade dos dados

- Lucas altera e acessa o schema `gateway` por meio do API Gateway;
- Gabriel altera o schema `estoque` pelo MS Estoque;
- Gabriel altera o schema `pagamento` pelo MS Pagamento;
- Gabriel altera o schema `promocoes` pelo MS Promoções;
- nenhum microsserviço consulta diretamente as tabelas de outro serviço;
- dados entre serviços trafegam somente por REST ou RabbitMQ.

### Simulações para trabalho paralelo

- Lucas utiliza catálogo e eventos fictícios para desenvolver React, Gateway e
  SSE sem aguardar os serviços reais;
- Gabriel utiliza produtores de teste para enviar eventos aos microsserviços
  sem aguardar a interface;
- o Mock de Pagamento pode ser testado com chamadas HTTP manuais;
- cada simulação deve respeitar exatamente o contrato aprovado;
- a simulação é substituída pelo serviço real nos pontos de integração.

### Regra de integração

A integração deve acontecer ao final de cada ciclo, e não apenas no final do
projeto. Cada ponto de integração precisa validar pelo menos o caminho de
sucesso e um caminho de erro.

---

## Sequência de trabalho em paralelo

### Ciclo 1 - Fundação

**Lucas**

- conectar o Gateway ao PostgreSQL;
- implementar cadastro e login;
- preparar a navegação inicial do React.

**Gabriel**

- conectar o Estoque ao PostgreSQL;
- migrar os produtos do JSON;
- criar o endpoint REST interno do catálogo.

**Ponto de integração**

- Gateway consulta o catálogo do Estoque e o React exibe os produtos.

### Ciclo 2 - Pedido

**Lucas**

- criar o endpoint de pedidos;
- persistir pedido e itens;
- publicar `pedido.criado`.

**Gabriel**

- consumir o pedido no Estoque;
- reservar produtos;
- publicar o resultado da reserva.

**Ponto de integração**

- pedido criado pelo React altera corretamente o estoque.

### Ciclo 3 - Pagamento e tempo real

**Lucas**

- consumir atualizações no Gateway;
- atualizar os estados;
- implementar SSE no Gateway e React.

**Gabriel**

- implementar MS Pagamento;
- implementar o Mock e o webhook;
- implementar o fluxo da Entrega.

**Ponto de integração**

- aprovação ou recusa aparece em tempo real para o cliente correto.

### Ciclo 4 - Promoções

**Lucas**

- criar a tela de preferências;
- criar os endpoints públicos de inscrição e cancelamento.

**Gabriel**

- persistir os interesses;
- integrar a API externa;
- enviar e registrar os e-mails.

**Ponto de integração**

- o cliente controla as categorias e recebe apenas as promoções escolhidas.

### Ciclo 5 - Finalização

**Ambos**

- executar todos os cenários de teste;
- corrigir falhas de integração;
- revisar segurança e isolamento dos clientes;
- concluir README e diagramas;
- preparar e ensaiar a defesa.

---

## Fluxo de Git recomendado

Evitar uma branch permanente por pessoa. Criar uma branch curta por tarefa:

```text
feat/gateway-auth
feat/estoque-postgres
feat/order-sse
feat/payment-webhook
feat/promotions-email
```

Antes de trabalhar em arquivo compartilhado:

1. avisar o outro integrante;
2. sincronizar a branch com a base;
3. manter a alteração pequena;
4. abrir o pull request;
5. solicitar revisão do colega;
6. integrar somente depois dos testes.

## Regra para a defesa

A divisão serve para organizar a implementação, mas não deve limitar o
conhecimento. Ao final de cada ciclo, cada pessoa deve explicar ao colega:

- o que implementou;
- quais endpoints ou eventos criou;
- quais dados persiste;
- como o erro é tratado;
- como demonstrar o funcionamento.

Assim, qualquer integrante poderá defender o fluxo completo.
