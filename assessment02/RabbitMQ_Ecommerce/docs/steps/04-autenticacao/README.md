# Etapa 04 - Autenticação e telas de acesso

Esta etapa implementa cadastro, login, identificação da sessão e logout no API
Gateway, além das respectivas telas React. Cada cliente passa a possuir uma conta
no PostgreSQL, que será utilizada para isolar seus pedidos nas próximas etapas.

## Resultado

- senhas armazenadas somente como hash bcrypt;
- sessão representada por JWT com expiração;
- JWT enviado em cookie `HttpOnly`;
- usuários persistidos em `gateway.users`;
- quatro endpoints REST de autenticação;
- telas responsivas de login e cadastro;
- sessão restaurada ao atualizar a página;
- área inicial exibida somente para usuário autenticado.

## Fluxo geral

```text
React
  -> API Gateway
     -> validação dos dados
     -> bcrypt / JWT
     -> PostgreSQL (gateway.users)
  <- cookie de sessão + usuário público
```

## Backend

### Arquivos

| Arquivo | Responsabilidade |
|---|---|
| `auth/password.go` | Gerar e comparar hashes bcrypt |
| `auth/token.go` | Gerar e validar JWT |
| `auth/user_repository.go` | Executar consultas de usuários no PostgreSQL |
| `auth/http_handler.go` | Validar requisições e responder às rotas REST |
| `cmd/gateway/main.go` | Montar e conectar as dependências |
| `ms-gateway/handler.go` | Registrar as rotas de autenticação no Gateway |

Os arquivos da pasta `auth` estão dentro de
`microservices/ms-gateway/auth`.

### Rotas

| Método | Rota | Resposta esperada |
|---|---|---|
| `POST` | `/api/auth/register` | HTTP 201 e usuário criado |
| `POST` | `/api/auth/login` | HTTP 200, usuário e cookie |
| `GET` | `/api/auth/me` | HTTP 200 com o usuário da sessão |
| `POST` | `/api/auth/logout` | HTTP 204 e remoção do cookie |

### Senhas

O bcrypt gera um salt aleatório junto com o hash. A implementação exige no mínimo
8 caracteres e respeita o limite técnico de 72 bytes do algoritmo. A senha em
texto aberto nunca é salva no banco nem enviada na resposta.

### JWT e cookie

O token utiliza HS256, emissor `rabbitmq-ecommerce` e validade de 24 horas. Seu
campo `subject` contém somente o UUID do usuário. O segredo deve possuir pelo
menos 32 bytes e vir de `JWT_SECRET`.

O cookie se chama `session` e utiliza:

- `HttpOnly`, impedindo leitura direta pelo JavaScript;
- `SameSite=Lax`, reduzindo envios a partir de sites externos;
- `Path=/`, permitindo uso em toda a aplicação;
- duração de 24 horas.

O atributo `Secure` fica desativado no ambiente HTTP local. Ele deve ser ativado
se a aplicação for publicada com HTTPS.

### Persistência

O repositório oferece apenas as operações necessárias ao trabalho:

- `Create`;
- `FindByEmail`;
- `FindByID`.

Os valores são enviados ao PostgreSQL por parâmetros SQL. E-mails são
normalizados para letras minúsculas. Violações de e-mail único são convertidas
em HTTP 409.

### Dependências

```text
github.com/golang-jwt/jwt/v5
golang.org/x/crypto/bcrypt
```

## Frontend

### Arquivos

| Arquivo | Responsabilidade |
|---|---|
| `frontend/src/api.ts` | Chamadas de cadastro, login, sessão e logout |
| `frontend/src/App.tsx` | Fluxo visual de autenticação e área do cliente |
| `frontend/src/styles.css` | Identidade visual e responsividade |

Todas as chamadas utilizam `credentials: 'include'`, permitindo que o navegador
receba e envie o cookie de sessão. Durante o desenvolvimento, o proxy do Vite
encaminha `/api` para `http://localhost:8080`.

### Padrão visual

O padrão inicial do projeto foi preservado e ampliado:

- fundo claro e painel verde escuro;
- detalhes terracota e dourado;
- títulos grandes;
- botões arredondados;
- formulário com mensagens de erro;
- estado de carregamento;
- adaptação para desktop, tablet e celular.

O cadastro realiza login automaticamente. Ao atualizar a página, o React consulta
`GET /api/auth/me` e recupera a sessão existente.

## Como executar

Inicie a infraestrutura:

```powershell
docker compose up -d
```

No terminal do Gateway, defina um segredo local e execute:

```powershell
$env:JWT_SECRET = "student-project-jwt-secret-2026-local"
go run ./cmd/gateway
```

Em outro terminal:

```powershell
cd frontend
npm run dev
```

Acesse `http://localhost:5173`.

## Validações realizadas

| Cenário | Resultado |
|---|---:|
| Cadastro válido | HTTP 201 |
| Login válido | HTTP 200 |
| Consulta `/me` com sessão | HTTP 200 |
| Logout | HTTP 204 |
| Consulta `/me` após logout | HTTP 401 |
| E-mail duplicado | HTTP 409 |
| Senha incorreta | HTTP 401 |
| Testes Go | Aprovados |
| Build React | Aprovado |
| Fluxo no navegador | Aprovado |
| Layout responsivo | Aprovado |

## Escopo acadêmico

A solução inclui os controles essenciais sem introduzir complexidade incompatível
com o trabalho. Não foram adicionados refresh token, OAuth, Redis, ORM ou controle
avançado de papéis.

Nas próximas etapas, o UUID da sessão será usado para garantir que cada cliente
visualize somente seus próprios pedidos. O projeto continuará incorporando REST,
HATEOAS, método `QUERY`, RabbitMQ, SSE e os microsserviços previstos no enunciado.

## Orientação para Gabriel

O frontend não acessa o JWT diretamente. Para consumir uma rota autenticada,
deve usar o mesmo cliente HTTP com cookies habilitados. Microsserviços internos
não devem receber a senha do cliente; quando necessário, o Gateway propagará
somente o identificador do usuário nos contratos já definidos.
