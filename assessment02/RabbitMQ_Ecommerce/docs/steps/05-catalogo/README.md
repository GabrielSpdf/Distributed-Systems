# Etapa 05 - Catálogo no Gateway e React

A parte de Lucas foi implementada e validada com um Estoque simulado.
A integração com o MS Estoque real de Gabriel permanece pendente.

## Responsabilidades

- Lucas: cliente HTTP do Estoque, rota pública no Gateway e catálogo React.
- Gabriel: endpoint interno, catálogo persistido e controle de quantidades.

## Comunicação

```text
React -> GET /api/products no Gateway
      -> GET /internal/products no Estoque
```

Os endpoints utilizam filtros opcionais `category` e `available`.
O contrato de resposta é:

```json
{
  "products": [
    {
      "id": "PROD-001",
      "name": "Arroz 5 kg",
      "category": "alimentos",
      "price": 29.90,
      "quantity": 30
    }
  ],
  "total": 1
}
```

## Implementação de Lucas

- `utils/config`: variável `STOCK_SERVICE_URL`, padrão local
  `http://localhost:8081`.
- `microservices/ms-gateway/catalog/client.go`: comunicação com o Estoque,
  contexto de cancelamento, timeout de cinco segundos e leitura do JSON.
- `catalog/http_handler.go`: rota pública, validação de filtros e respostas.
- `cmd/gateway/main.go`: criação e conexão do cliente e handler.
- `frontend/src/api.ts`: tipos e função `getProducts`.
- `frontend/src/Catalog.tsx`: catálogo, busca por nome e filtros.
- `frontend/src/styles.css`: cards e layout responsivo com a identidade existente.
- `cmd/mock-stock/main.go`: servidor de demonstração para desenvolvimento local.

A categoria e a disponibilidade são filtradas pelo serviço de Estoque.
A busca por nome acontece no React sobre os produtos recebidos.
O catálogo é exibido na área autenticada, mas a rota pública de produtos
não exige autenticação.

## Comportamentos HTTP

| Situação | Resposta |
|---|---|
| Catálogo consultado | HTTP 200 |
| Disponibilidade diferente de true ou false | HTTP 400 |
| Falha na comunicação ou resposta do Estoque | HTTP 503 |

Um catálogo vazio é diferente de um serviço indisponível. A interface possui
estados separados de carregamento, erro e lista vazia, além de nova tentativa.

## Produtos de demonstração

O mock possui 14 produtos distribuídos entre alimentos, limpeza e eletrônicos.
Os IDs PROD-001, PROD-002 e PROD-003 preservam os produtos da migration inicial.
Os demais devem ser alinhados com Gabriel antes do cadastro na base real.
O carregador USB-C tem quantidade zero para demonstrar indisponibilidade.

Esses dados ficam somente na memória do mock e não alteram o PostgreSQL.
O mock não implementa reserva, pedidos ou eventos RabbitMQ.

## Execução local

Na raiz do projeto, execute a infraestrutura:

```powershell
docker compose up -d
```

No terminal do Gateway:

```powershell
$env:JWT_SECRET = "student-project-jwt-secret-2026-local"
$env:STOCK_SERVICE_URL = "http://localhost:8081"
go run ./cmd/gateway
```

Esse segredo é apenas um exemplo para demonstração local.

Em outro terminal, execute o mock:

```powershell
go run ./cmd/mock-stock
```

Em um terceiro terminal:

```powershell
cd frontend
npm run dev
```

Acesse http://localhost:5173, entre e selecione "Explorar produtos".

## Verificações realizadas

- testes da configuração aprovados;
- testes do cliente HTTP com Estoque temporário aprovados;
- teste da rota pública com resposta simulada aprovado;
- testes Go aprovados na integração do Gateway;
- HTTP 503 observado com o Estoque desligado;
- HTTP 400 observado com available=talvez;
- build React aprovado durante o desenvolvimento;
- interface e catálogo com 14 produtos aprovados por Lucas.

Antes do commit, repetir `go test ./...` e `npm run build` para verificar
o conjunto final.

## Integração com Gabriel

1. Confirmar a porta do endpoint interno.
2. Encerrar o mock para liberar a porta, se for a mesma.
3. Iniciar o MS Estoque real.
4. Ajustar STOCK_SERVICE_URL no terminal do Gateway e reiniciá-lo.
5. Verificar produtos, filtros, catálogo vazio e indisponibilidade.
6. Alinhar os produtos adicionais com os dados persistidos.

O frontend e o cliente do Gateway devem continuar utilizando o contrato aprovado.
Esta etapa ainda não adiciona carrinho nem criação de pedidos.
