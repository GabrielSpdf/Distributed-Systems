# Etapa 03 - Integração entre Gateway e PostgreSQL

Esta etapa conecta o API Gateway ao PostgreSQL por meio de um pool de conexões e
transforma a rota `GET /healthz` em uma verificação real da disponibilidade do
banco de dados.

## Objetivos

- carregar a URL do PostgreSQL pela configuração da aplicação;
- criar e validar um pool de conexões na inicialização do Gateway;
- encerrar o pool de forma segura junto com a aplicação;
- informar `healthy` apenas quando o banco estiver acessível;
- retornar HTTP `503 Service Unavailable` quando o banco estiver indisponível;
- testar o comportamento saudável e o cenário de falha sem depender do banco real.

## Dependência utilizada

O projeto utiliza `pgx/v5` e o pacote `pgxpool`:

```text
github.com/jackc/pgx/v5/pgxpool
```

O `pgx` é o driver usado para comunicação com PostgreSQL. O `pgxpool` mantém um
conjunto reutilizável de conexões, evitando criar uma conexão nova para cada
requisição HTTP.

## Arquivos envolvidos

| Arquivo | Responsabilidade |
|---|---|
| `utils/database/postgres.go` | Configurar, abrir e validar o pool do PostgreSQL |
| `cmd/gateway/main.go` | Inicializar o pool e entregá-lo ao handler HTTP |
| `microservices/ms-gateway/handler.go` | Consultar o banco na rota `/healthz` |
| `microservices/ms-gateway/handler_test.go` | Testar respostas `200` e `503` com um stub |
| `go.mod` e `go.sum` | Registrar a dependência e suas versões |

## Como a conexão funciona

A função `database.Connect` recebe um `context.Context` e a URL definida em
`DATABASE_URL`. Antes de criar o pool, ela rejeita uma URL vazia e interpreta a
configuração com `pgxpool.ParseConfig`.

O pool foi configurado com:

| Parâmetro | Valor | Finalidade |
|---|---:|---|
| `MinConns` | 1 | Manter ao menos uma conexão disponível |
| `MaxConns` | 10 | Limitar o consumo de conexões do banco |
| `MaxConnIdleTime` | 5 minutos | Descartar conexões ociosas antigas |
| `MaxConnLifetime` | 30 minutos | Renovar conexões periodicamente |
| timeout inicial | 5 segundos | Evitar inicialização bloqueada indefinidamente |

Criar um pool não garante, sozinho, que o PostgreSQL esteja acessível naquele
momento. Por isso, a função chama `Ping` antes de devolver o pool. Se a validação
falhar, o pool é fechado e o Gateway não inicia como se estivesse saudável.

No ponto de entrada do Gateway, `defer databasePool.Close()` garante a liberação
das conexões quando o processo for encerrado.

## Health check

A rota utilizada é:

```http
GET /healthz
```

Em cada chamada, o handler cria um contexto com limite de dois segundos e executa
`Ping` no banco.

Quando o PostgreSQL responde:

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"service":"ms-principal-api-gateway","status":"healthy"}
```

Quando o PostgreSQL está indisponível:

```http
HTTP/1.1 503 Service Unavailable
Content-Type: application/json

{"service":"ms-principal-api-gateway","status":"unhealthy"}
```

O código `503` indica que o processo HTTP está ativo, mas não está pronto para
executar normalmente porque uma dependência essencial falhou.

A rota `GET /` tem outra finalidade: ela apenas identifica o serviço e retorna
`available`. Ela não verifica o banco e, portanto, não substitui `/healthz`.

## Por que existe a interface `DatabasePinger`

O handler depende apenas desta operação:

```go
type DatabasePinger interface {
	Ping(ctx context.Context) error
}
```

O pool real do `pgx` satisfaz essa interface. Nos testes, ele é substituído por um
stub que pode simular sucesso ou erro. Assim, os testes do Gateway permanecem
rápidos e não exigem um container PostgreSQL em execução.

Os testes cobrem:

- HTTP `200` quando `Ping` funciona;
- HTTP `503` quando `Ping` retorna erro;
- cabeçalho JSON da resposta;
- preflight CORS do frontend.

## Como executar

Inicie a infraestrutura:

```powershell
docker compose up -d
docker compose ps
```

Execute os testes:

```powershell
go test ./...
```

Inicie o Gateway:

```powershell
go run ./cmd/gateway
```

Em outro PowerShell, consulte a saúde:

```powershell
curl.exe -i http://localhost:8080/healthz
```

## Teste manual de falha e recuperação

Com o Gateway em execução, pare somente o PostgreSQL:

```powershell
docker compose stop postgres
curl.exe -i http://localhost:8080/healthz
```

O resultado esperado é HTTP `503` com estado `unhealthy`.

Religue o banco e aguarde o container ficar saudável:

```powershell
docker compose start postgres
docker compose ps
```

Repita a consulta:

```powershell
curl.exe -i http://localhost:8080/healthz
```

O resultado deve voltar para HTTP `200` com estado `healthy`, sem reiniciar o
Gateway. Isso demonstra que o pool consegue restabelecer conexões depois que o
banco retorna.

## Resultado validado pela dupla

Durante esta etapa foram confirmados:

- todos os pacotes aprovados por `go test ./...`;
- HTTP `200 OK` com o PostgreSQL disponível;
- HTTP `503 Service Unavailable` após parar o PostgreSQL;
- retorno para HTTP `200 OK` após religar o PostgreSQL;
- recuperação sem reinicialização do Gateway.

## Orientação para Gabriel

Os demais microsserviços que precisarem do PostgreSQL podem reutilizar
`database.Connect`, cada um com sua própria configuração e ciclo de vida do pool.
Não se deve compartilhar uma conexão individual entre processos. Para testar um
handler, prefira uma interface pequena, como `DatabasePinger`, em vez de depender
diretamente de um banco real.

Esta etapa não cria repositórios de domínio nem consultas de pedidos, pagamentos
ou estoque. Ela fornece somente a infraestrutura de conexão e de monitoramento;
as operações de negócio serão adicionadas nas etapas específicas.
