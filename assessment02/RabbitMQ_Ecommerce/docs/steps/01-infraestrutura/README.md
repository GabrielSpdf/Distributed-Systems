# Etapa 01 - Infraestrutura local

Este guia registra a configuração inicial da Avaliação 02. Ao final, PostgreSQL,
RabbitMQ, API Gateway e frontend devem estar disponíveis localmente.

## Pré-requisitos

- Go 1.27.1 ou compatível;
- Node.js 22 ou compatível;
- Docker Desktop com o mecanismo em execução;
- Git;
- PowerShell.

## 1. Acessar o projeto

```powershell
cd "C:\Users\lucas\OneDrive\Desktop\Faculdade\11º Período\Sistemas_Distribuidos\Atividade_02\Distributed-Systems\assessment02\RabbitMQ_Ecommerce"
```

## 2. Criar a configuração local

```powershell
Copy-Item .env.example .env
```

Abra o arquivo `.env` e substitua o valor de `JWT_SECRET` por um segredo longo.
O arquivo `.env` não deve ser enviado ao Git.

## 3. Iniciar PostgreSQL e RabbitMQ

Confirme primeiro que o Docker Desktop indica que o mecanismo está em execução.

```powershell
docker compose up -d
docker compose ps
```

Os containers `postgres` e `rabbitmq` devem aparecer como `healthy`.

Portas utilizadas:

| Serviço | Endereço |
|---|---|
| PostgreSQL | `localhost:5432` |
| RabbitMQ AMQP | `localhost:5672` |
| RabbitMQ Management | `http://localhost:15672` |

O usuário e a senha padrão do RabbitMQ no ambiente local são `guest`.

## 4. Verificar o banco

Liste os schemas:

```powershell
docker compose exec postgres psql -U ecommerce -d ecommerce -c "\dn"
```

Devem existir os schemas:

```text
gateway
estoque
pagamento
promocoes
```

Liste as tabelas do Gateway:

```powershell
docker compose exec postgres psql -U ecommerce -d ecommerce -c "\dt gateway.*"
```

## 5. Executar o API Gateway

Em outro terminal, na raiz do projeto:

```powershell
go run ./cmd/gateway
```

Verificações:

- `http://localhost:8080/` retorna o serviço com estado `available`;
- `http://localhost:8080/healthz` retorna o serviço com estado `healthy`.

## 6. Executar o frontend

Em outro terminal:

```powershell
cd frontend
npm install
npm run dev
```

Abra `http://localhost:5173`.

## Comandos úteis

Parar os containers sem apagar os dados:

```powershell
docker compose stop
```

Iniciar novamente:

```powershell
docker compose start
```

Ver os logs:

```powershell
docker compose logs -f
```

Remover apenas os containers e a rede:

```powershell
docker compose down
```

Não utilize `docker compose down -v` sem intenção explícita, pois a opção `-v`
remove também os volumes e os dados armazenados.

## Critério de conclusão

- PostgreSQL saudável;
- RabbitMQ saudável;
- schemas criados no PostgreSQL;
- API Gateway respondendo;
- frontend acessível no navegador.

Com essa etapa concluída, o próximo trabalho é conectar o Gateway ao PostgreSQL
e implementar o cadastro de usuários.

