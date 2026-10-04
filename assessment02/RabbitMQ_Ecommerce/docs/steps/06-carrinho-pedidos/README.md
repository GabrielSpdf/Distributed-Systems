# Etapa 06 - Carrinho e pedidos (em andamento)

Responsável principal: Lucas. Branch: `feat/cart-orders`.

## Progresso atual

O carrinho, a criação de pedidos e as consultas por usuário foram implementados.
Os pedidos são persistidos no PostgreSQL com o estado inicial PENDENTE.
A publicação RabbitMQ e o processamento pelos demais serviços ainda estão pendentes.

## Arquivos envolvidos

| Arquivo | Responsabilidade |
|---|---|
| `frontend/src/App.tsx` | Estado do carrinho, adição, remoção e alteração de quantidade |
| `frontend/src/cartTypes.ts` | Tipo compartilhado CartItem |
| `frontend/src/Cart.tsx` | Lista, controles de quantidade, subtotais e total |
| `frontend/src/Catalog.tsx` | Botão de adição e quantidade selecionada por produto |
| `frontend/src/styles.css` | Carrinho lateral, responsividade e ajustes visuais |
| `frontend/src/api.ts` | Requisições de criação, listagem e detalhes |
| `frontend/src/Orders.tsx` | Pedidos do usuário e detalhes dentro do cartão selecionado |
| `microservices/ms-gateway/auth/middleware.go` | Autenticação das rotas pelo cookie de sessão |
| `microservices/ms-gateway/orders/` | Modelos, validação, cálculo, persistência e rotas HTTP |
| `migrations/002_order_sequence.sql` | Sequência dos identificadores públicos dos pedidos |
| `cmd/gateway/main.go` | Montagem das dependências dos pedidos |
| `microservices/ms-gateway/handler.go` | Registro das rotas protegidas |

## Comportamento

- Adicionar novamente um produto aumenta a quantidade do mesmo item.
- A seleção é limitada pelo estoque conhecido no catálogo.
- O controle de redução para em uma unidade; o botão Remover exclui o item.
- O catálogo informa estoque, quantidade no carrinho e quantidade restante para adicionar.
- O botão de adição é desabilitado quando o limite é atingido.
- Subtotais e total estimado são calculados em centavos inteiros.
- O logout limpa o carrinho.
- O carrinho fica na memória e é perdido ao atualizar a página.
- No desktop, o carrinho aparece na lateral com posição sticky.
- Em telas menores, aparece abaixo do catálogo.

Adicionar ao carrinho não reserva nem reduz o estoque real. A reserva será
executada pelo MS Estoque após receber o evento de criação do pedido.
O backend consulta os preços novamente; o total do frontend é estimativo.

## Pedidos e autenticação

- `POST /api/orders` recebe apenas os identificadores e quantidades dos itens.
- O usuário é identificado pela sessão, não por um identificador enviado no corpo.
- O backend valida os itens e calcula o total em centavos inteiros.
- Pedido, itens e histórico inicial são gravados na mesma transação.
- A sequência gera identificadores públicos como PED-001; o UUID interno não é exposto.
- `GET /api/orders` lista somente pedidos do usuário autenticado.
- `GET /api/orders/{orderId}` consulta um pedido do mesmo usuário.
- Pedido inexistente ou pertencente a outro usuário retorna 404.
- O link HATEOAS `self` aponta para a consulta de detalhes. Outras ações ainda não foram implementadas.
- Após criar um pedido, o frontend limpa o carrinho e atualiza a lista.
- A confirmação desaparece com transição suave e também é limpa ao iniciar outra seleção.
- Os detalhes aparecem dentro do cartão correspondente. Ver detalhes fica oculto enquanto o detalhe está aberto.

A migration 002 foi aplicada manualmente no banco local existente. Em um volume
já inicializado, reiniciar os contêineres não executa novamente os scripts de inicialização.

## Validação

Um build anterior do carrinho passou. O build após os últimos ajustes de pedidos
e detalhes ainda precisa ser confirmado.
Lucas confirmou os ajustes visuais no navegador e apresentou testes Go aprovados.

Validações manuais realizadas:

- criação sem sessão: 401;
- criação autenticada: 201;
- PED-001 com dois arrozes e três detergentes: total R$ 74,50;
- pedido, itens e histórico PENDENTE conferidos no PostgreSQL;
- listagem do proprietário: 200 com seu pedido;
- listagem de outro usuário: 200 com lista vazia;
- detalhes pelo proprietário: 200 com link self;
- detalhes por outro usuário: 404.

Antes de concluir a etapa, conferir:

- limites dos controles + e -;
- atualização conjunta de quantidade, subtotal, total e aviso do catálogo;
- remoção liberando novamente a quantidade para seleção;
- limpeza após logout;
- disposição do carrinho em tela estreita.

## Próximos passos

1. Confirmar o build final do frontend e revisar as alterações.
2. Integrar a publicação de pedido.criado usando o contrato aprovado e tratar falhas.
3. Integrar reserva e respostas do MS Estoque com Gabriel.
4. Atualizar o estado do pedido a partir dos eventos recebidos.
5. Acrescentar as ações HATEOAS quando suas rotas estiverem implementadas.
6. Alinhar o formato dos erros HTTP ao contrato de integração.
7. Testar o fluxo integrado e registrar os resultados antes de concluir a etapa.

Os tipos antigos de eventos usam itens com produto aninhado. O contrato novo
de pedido.criado usa product_id, product_name, quantity e unit_price diretamente
em cada item, além de currency no pedido. Essa diferença deve ser resolvida em
conjunto sem alterar silenciosamente os consumidores existentes.

## Orientação para Gabriel

O carrinho não modifica as tabelas do Estoque. Gabriel continua responsável
pela validação e reserva definitiva das quantidades através dos eventos.
O mock do catálogo permanece um recurso local de demonstração até a integração
com o serviço real.

## Git

As alterações desta etapa ainda não foram commitadas.
`frontend/tsconfig.app.tsbuildinfo` é cache gerado pelo TypeScript e sua
alteração não deve entrar no commit funcional.
