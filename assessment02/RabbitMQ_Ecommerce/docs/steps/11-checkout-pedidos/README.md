# Etapa 11 - Link de checkout dos pedidos

## Base e escopo

Branch `feat/order-checkout`, criada a partir de `dd7fa61`, após a integração
do SSE pelo PR #21. Lucas implementa a apresentação do checkout no Gateway
e no React. A tela de mock de pagamento e seu processamento pertencem à
integração com Gabriel e não foram implementados nesta etapa.

## Fluxo e implementação

1. O pedido recebe `pedido.estoque_ok` e passa a `ESTOQUE_CONFIRMADO`.
2. O serviço de pagamento publica `pagamento.checkout_disponivel`, assinado
   como `ms-pagamento`, com `order_id`, `customer_id`, `charge_id`,
   `checkout_url` e `expires_at`.
3. O consumidor existente valida o evento, persiste os dados do checkout e
   muda o estado para `AGUARDANDO_PAGAMENTO`. O SSE avisa após o commit.
4. O React consulta os pedidos e mostra o link disponibilizado pelo Gateway;
   receber o evento não abre automaticamente uma aba.

`Order` expõe `checkout_expires_at` quando presente. As consultas de listagem
e detalhes carregam o campo já criado na migration 003; não há migration nova.

O Gateway inclui `_links.checkout`, com método `GET`, somente no estado
`AGUARDANDO_PAGAMENTO`, com validade futura e URL HTTP/HTTPS absoluta,
hostname não vazio e sem credenciais na URL.

O React revalida estado, método, URL e validade antes de mostrar “Ir para
pagamento”. O link abre em nova aba com `noopener noreferrer`. Um efeito
independente atualiza o relógio local a cada segundo e limpa o intervalo ao
desmontar o componente. Ao expirar, o botão desaparece e a interface informa
que o checkout expirou, sem recarregar. O clique também verifica a validade
com o horário atual.

O simulador `cmd/mock-order-status` mantém o envio de estoque e passa a aceitar
`-checkout`, `-checkout-url`, `-charge` e `-valid-for`. No modo checkout usa
a chave existente de `ms-pagamento`. `-repeat` repete o mesmo envelope;
`-tamper` permanece exclusivo do estoque.

## Validação automatizada

- `TestOrderCheckoutLink`: link HTTPS e mock HTTP local; validade ausente,
  zero ou expirada; URL vazia, relativa, inválida, com esquema não permitido
  ou credenciais; estados em que checkout não é permitido; remoção de link
  antigo e preservação das regras de `self` e cancelamento.
- `TestApplyStatusEventIntegration`: consulta da URL e validade persistidas
  tanto nos detalhes quanto na listagem, além dos testes existentes de status,
  inbox e outbox.
- A execução final de `go test ./... -count=1`, com `TEST_DATABASE_URL`
  apontando para o banco isolado, passou. O build do frontend também passou.
- `git diff --check` não reportou problemas de espaços; apenas avisos de
  conversão de finais de linha LF para CRLF no Windows.

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
go test ./microservices/ms-gateway/orders -run 'TestOrderCheckoutLink|TestApplyStatusEventIntegration' -v -count=1
go test ./... -count=1
Push-Location frontend
npm run build
Pop-Location
git diff --check
```

Use somente `ecommerce_events_test` para os testes de integração, não o banco
da aplicação.

## Validação manual

O usuário confirmou o fluxo do `PED-039`, de R$ 4,90: confirmação de estoque,
checkout disponível sem recarregar, botão para nova aba e indicação de
expiração após dois minutos. O checkout foi enviado duas vezes usando
o mesmo envelope assinado.

Com Gateway, catálogo simulado e frontend ativos, os comandos usados foram:

```powershell
$env:RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
go run ./cmd/mock-order-status `
    -order PED-039 `
    -customer c537d2d6-f125-4119-965e-3854562c8729 `
    -total 4.90

go run ./cmd/mock-order-status `
    -order PED-039 `
    -customer c537d2d6-f125-4119-965e-3854562c8729 `
    -total 4.90 `
    -checkout `
    -charge "checkout-teste-PED-039" `
    -checkout-url "http://localhost:8083/checkout/checkout-teste-PED-039" `
    -valid-for 2m `
    -repeat 2
```

Para repetir, crie outro pedido e substitua pedido, cliente, total e cobrança.
O `PED-039` já avançou de estado; esses comandos não reiniciam seu fluxo.

## Limitações e integração pendente

- A URL local é um destino de teste. Esta etapa não comprova que exista uma
  página de pagamento na porta 8083, nem aprovação, recusa ou webhook real.
- Ocultar um link expirado não muda o estado, não publica compensação e não
  devolve estoque. Cancelamento automático por expiração ainda precisa ser
  implementado e validado no fluxo integrado.
- O relógio do navegador orienta a apresentação; o serviço de pagamento
  deverá impor a validade e as regras da cobrança no servidor.
- A validação de URL restringe formato e esquema, mas não é uma lista de
  domínios de pagamento autorizados.
- A apresentação foi validada manualmente; o build não substitui testes
  automatizados de interação do React.

## Estado

Implementação e validação local concluídas nesta branch. Commit e PR desta
etapa ainda pendentes. Integração real com Gabriel permanece pendente.
