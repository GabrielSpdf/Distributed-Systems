# Etapa 12 - Preferências, ofertas e e-mail

## Escopo

Fluxo acadêmico implementado por Lucas em `feat/promotion-preferences`.
Pagamento e checkout real não fazem parte desta entrega.

1. React envia a preferência do cliente autenticado ao Gateway.
2. Gateway grava preferência e outbox na mesma transação e publica
   `interesse.promocao`, assinado por `ms-principal`.
3. MS Promoções valida a assinatura e registra inbox e interesses em seu schema.
4. Publicador sorteia produto/desconto e publica oferta assinada por `ms-promocoes`.
5. Consumidor valida a oferta e grava envios pendentes para interesses ativos.
6. Executor revalida o interesse e envia pelo Resend, registrando as tentativas.

Cada serviço acessa somente seu schema. O e-mail vem do cadastro, não da requisição.

| Evento | Exchange | Fila |
|---|---|---|
| `interesse.promocao` | `eCommerce` (direct) | `promocoes.interesses` |
| `promocao.categoria.limpeza` | `Promocoes` (topic) | `promocoes.ofertas` |
| `promocao.categoria.alimentos` | `Promocoes` (topic) | `promocoes.ofertas` |
| `promocao.categoria.eletronicos` | `Promocoes` (topic) | `promocoes.ofertas` |

A fila de ofertas usa o binding `promocao.categoria.*`.

## API autenticada

- `GET /api/promotion-interests`: categorias ativas do próprio usuário.
- `POST /api/promotion-interests`: recebe `{"categories":["alimentos"]}`;
  retorna 202 quando altera e 200 quando não há alteração.
- `DELETE /api/promotion-interests/alimentos`: cancela; repetir retorna 204,
  sem criar outro evento desnecessário.
- Sem autenticação: 401. Um usuário não acessa as escolhas de outro.

## Preparação

Todos os comandos partem da raiz de `assessment02/RabbitMQ_Ecommerce`.

```powershell
docker compose up -d
docker compose ps
```

As migrations 001 a 006 são pré-requisito. Aplicar 007 a 009 uma vez em cada banco
de desenvolvimento/teste. Não reaplicar às cegas: nem todos os comandos são
idempotentes. O Compose não atualiza volumes existentes automaticamente.

- 007: preferências e outbox do Gateway.
- 008: inbox dos interesses no MS Promoções.
- 009: jobs e logs das tentativas de e-mail.

Para uma migration ainda não aplicada, usar `Get-Content -Raw` com
`docker compose exec -T postgres psql -U ecommerce -d NOME_DO_BANCO -v ON_ERROR_STOP=1`.
Banco da aplicação: `ecommerce`; banco isolado: `ecommerce_events_test`.

Adicionar somente as novas chaves, uma única vez, preservando as existentes:

```powershell
go run ./cmd/keygen -promotions-only
Test-Path keys/ms-promocoes/private.pem
Test-Path keys/ms-promocoes/public.pem
```

O gerador recusa sobrescrever arquivos. Não remover `keys` nem versionar chaves.

## Processos da demonstração

Manter Gateway e frontend funcionando. Gateway também exige `JWT_SECRET` com
pelo menos 32 bytes; preservar o mesmo segredo para manter as sessões.
Configurar as variáveis em cada terminal que as utiliza. Elas não são herdadas
de outro terminal, e a aplicação não carrega `.env` automaticamente.

```powershell
$env:RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
$env:DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce?sslmode=disable"
$env:RESEND_FROM = "Event Market <onboarding@resend.dev>"
$env:RESEND_TEST_RECIPIENT = Read-Host "Digite o e-mail autorizado para o teste"
```

Usar essas credenciais somente localmente. Executar um processo por terminal:

```powershell
# Sincroniza preferências; não envia e-mail.
go run ./cmd/promocoes -interests
```

```powershell
# Recebe ofertas e grava jobs; não envia e-mail.
go run ./cmd/promocoes -offers
```

Somente no terminal do executor, carregar a chave com entrada oculta:

```powershell
$resendSecret = Read-Host "Cole a chave atual do Resend" -AsSecureString
$resendPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($resendSecret)
try {
    $env:RESEND_API_KEY = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($resendPointer)
} finally {
    [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($resendPointer)
    $resendSecret.Dispose()
    Remove-Variable resendSecret, resendPointer
}

# ATENÇÃO: processa pendências e envia e-mails reais.
go run ./cmd/promocoes -emails
```

Não colocar a chave no texto do prompt, nos arquivos, logs ou Git. A entrada é
oculta, mas a chave continua no ambiente do processo até encerrar o terminal.
Os modos `-interests`, `-offers`, `-emails` e `-test-email` são exclusivos.
O consumidor `-offers` não precisa da chave do Resend.

## Publicação manual e automática

Usa `data/inventory.json`, não o serviço real de Estoque. Sorteia desconto de
5% a 30%, arredonda o preço para centavos e não modifica produtos ou estoque.

```powershell
# Sorteia uma oferta da categoria e encerra.
go run ./cmd/publish-promotions -once -category alimentos

# Uma oferta imediatamente; depois, a cada 60 segundos entre todas as categorias.
go run ./cmd/publish-promotions -interval 60s

# Automático focado na categoria do cliente.
go run ./cmd/publish-promotions -interval 60s -category alimentos

# Duas cópias com mesmo ID, conteúdo e assinatura.
go run ./cmd/publish-promotions -once -category alimentos -repeat 2
```

Categorias: `alimentos`, `limpeza`, `eletronicos`. Intervalo padrão 60 segundos,
sempre positivo. Repetições de 1 a 10, padrão 1. Parar com Ctrl+C.
Parar o publicador não apaga jobs já criados: parar também `-emails` para não enviá-los.

Não usar `go run ./cmd/promocoes` sem flags neste roteiro: é o gerador legado,
com intervalo de 10 segundos e ofertas não assinadas, rejeitadas pelo novo consumidor.

## Roteiro curto da apresentação

1. Entrar/cadastrar com o e-mail configurado e ativar Alimentos na interface.
2. Com `-interests` ativo, esperar `active = true` no MS Promoções.
3. Com `-emails` parado e `-offers` ativo, publicar uma oferta manual.
4. Mostrar `[PROCESSADO]` e job `PENDING` com zero tentativas no banco.
5. Iniciar `-emails`, mostrar `SENT` e conferir recebimento na caixa de entrada.
6. Parar `-emails`; mostrar duas ofertas automáticas com 60 segundos de diferença;
   parar o publicador após a segunda.
7. Cancelar Alimentos pela interface, esperar `active = false` no MS Promoções;
   publicar oferta e mostrar `novos envios=0`.
8. Reativar Alimentos, esperar sincronização e publicar com `-repeat 2`;
   mostrar mesmo ID com `novos envios=1` e depois `novos envios=0`.

Usar a interface para mudar preferências, não atualizações diretas no banco.
Consultas de acompanhamento (substituir o e-mail fictício pelo cadastrado):

```powershell
docker compose exec -T postgres psql -U ecommerce -d ecommerce `
    -c "SELECT category, active FROM promocoes.promotion_interests WHERE email = 'cliente@example.com' ORDER BY category;"

docker compose exec -T postgres psql -U ecommerce -d ecommerce `
    -c "SELECT promotion_event_id, category, subject, status, attempt_count FROM promocoes.email_jobs ORDER BY created_at DESC LIMIT 10;"

docker compose exec -T postgres psql -U ecommerce -d ecommerce `
    -c "SELECT job_id, attempt_number, status, provider_message_id FROM promocoes.email_logs ORDER BY created_at DESC LIMIT 10;"
```

## Garantias e limitações acadêmicas

- Outbox de interesses usa confirmação do RabbitMQ; inbox evita reaplicar o mesmo
  evento de interesse. Ofertas têm assinatura RSA-PSS/SHA-256 e validação de dados.
- Ofertas inválidas são rejeitadas sem reenfileirar; não há DLQ nesta etapa.
- ACK da oferta ocorre após gravar os envios. Falha no banco reenfileira a oferta
  e encerra o consumidor; reiniciar `-offers` para retomar.
- A unicidade `(promotion_event_id, user_id)` evita outro envio para o mesmo par.
  Não há inbox global de ofertas: repetir uma oferta após novas inscrições pode
  criar jobs para usuários que ainda não tinham registro.
- Publicador de ofertas usa mensagem persistente/fila durável, mas não espera
  publisher confirms. O log `[PUBLICADO]` sozinho não comprova confirmação do
  broker; mostrar também o consumidor/banco. Não há garantia de exatamente uma vez.
- Executor revalida o interesse e preserva a chave de idempotência nas tentativas.
  Cancelar interesse cancela jobs pendentes, mas não desfaz e-mails já enviados
  ou uma chamada ao provedor que já iniciou.
- Falhas temporárias são reagendadas: máximo de 12 tentativas e janela interna de
  23 horas. Limites/configuração alterada levam a `PENDING_REVIEW`.
- `SENT` significa aceito pelo Resend, não entrega confirmada. Não há webhook
  de confirmação de entrega de e-mail nesta etapa.
- Criação e envio dos jobs estão restritos a `RESEND_TEST_RECIPIENT`. Não afirmar
  envio liberado para todos os clientes; usar destinatário autorizado para o teste.
- Publicador/consumidor de ofertas precisam ser reiniciados em falhas de conexão.
  O modo `-interests` possui reconexão própria.

## Verificação final

```powershell
$env:TEST_DATABASE_URL = "postgres://ecommerce:ecommerce@localhost:5432/ecommerce_events_test?sslmode=disable"
$env:TEST_RABBITMQ_URL = "amqp://guest:guest@127.0.0.1:5672/"
go test ./... -count=1
go test ./microservices/ms-promocoes -run TestValidatePromotionEvent -v -count=1
go test ./microservices/ms-promocoes -run TestQueuePromotionEmailsIntegration -v -count=1
Push-Location frontend
npm run build
Pop-Location
git diff --check
```

Integrações podem ser puladas quando suas variáveis não estão configuradas.
Nunca usar `ecommerce` em `TEST_DATABASE_URL`. Não executar workers/outras
integrações simultaneamente no banco de teste durante a busca de pendências.

Foram demonstrados manualmente publicação periódica, fluxo pelo RabbitMQ,
recebimento real, cancelamento e duplicação. Isso não substitui teste automatizado
completo do consumidor de ofertas/publicador. Testes unitários não enviam e-mails reais.

`cmd/mock-promotion-email` cria um job diretamente no banco e serve apenas ao
diagnóstico. `-test-email` faz um envio de diagnóstico sem job e exige
`RESEND_TEST_IDEMPOTENCY_KEY`. Não usar esses auxiliares como demonstração do
fluxo completo via RabbitMQ.
