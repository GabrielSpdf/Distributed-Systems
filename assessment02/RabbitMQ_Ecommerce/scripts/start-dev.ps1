# Windows PowerShell 5.1+. Does not write secrets to disk or command-line arguments.
[CmdletBinding(SupportsShouldProcess = $true)]
param(
    [switch]$AbrirTerminais,
    [switch]$Verificar,
    [switch]$ComEmails,
    [switch]$PublicarOfertas,
    [ValidateSet('alimentos', 'limpeza', 'eletronicos')]
    [string]$Categoria = 'alimentos',
    [ValidatePattern('^[1-9][0-9]*(ms|s|m|h)$')]
    [string]$Intervalo = '60s',
    [ValidatePattern('^[1-9][0-9]*(ms|s|m|h)$')]
    [string]$ValidadeCheckout = '2m',
    # Internal option for the child terminals.
    [ValidateSet('', 'Estoque', 'Mock', 'Pagamento', 'Entrega', 'Interesses', 'Ofertas', 'Emails', 'Gateway', 'Frontend', 'Publicador')]
    [string]$Servico = ''
)
$ErrorActionPreference = 'Stop'
$ProjectRoot = Split-Path -Parent $PSScriptRoot
$ScriptFile = $PSCommandPath
$Settings = @('RABBITMQ_URL','DATABASE_URL','JWT_SECRET','PAYMENT_WEBHOOK_SECRET',
    'STOCK_SERVICE_URL','FRONTEND_ORIGIN','GATEWAY_PORT','STOCK_PORT','PAYMENT_PORT',
    'PAYMENT_WEBHOOK_URL','MOCK_PAYMENT_PORT','MOCK_PAYMENT_URL','MOCK_PAYMENT_PUBLIC_URL',
    'MOCK_CHECKOUT_TTL','RESEND_API_KEY','RESEND_FROM','RESEND_TEST_RECIPIENT')

function Test-Port([int]$Port) {
    foreach ($address in @('127.0.0.1', '::1')) {
        $family = if ($address -eq '::1') { [Net.Sockets.AddressFamily]::InterNetworkV6 } else { [Net.Sockets.AddressFamily]::InterNetwork }
        $client = New-Object System.Net.Sockets.TcpClient($family)
        try {
            $pending = $client.BeginConnect($address, $Port, $null, $null)
            if ($pending.AsyncWaitHandle.WaitOne(400)) {
                $client.EndConnect($pending)
                return $true
            }
        } catch { } finally { $client.Close() }
    }
    return $false
}
function Wait-Health([int]$Port) {
    $deadline = (Get-Date).AddSeconds(60)
    do {
        try {
            $path = if ($Port -eq 5173) { '/' } else { '/healthz' }
            $response = Invoke-WebRequest -UseBasicParsing -Uri "http://localhost:$Port$path" -TimeoutSec 2
            if ($response.StatusCode -eq 200) { return }
        } catch { }
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $deadline)
    throw "Porta $Port nao respondeu. Confira os terminais; nada sera encerrado automaticamente."
}
function Import-LocalEnvironment {
    $path = Join-Path $ProjectRoot '.env'
    if (-not (Test-Path -LiteralPath $path)) { return }
    foreach ($line in Get-Content -LiteralPath $path -Encoding UTF8) {
        if ($line -notmatch '^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$') { continue }
        $name, $value = $Matches[1], $Matches[2].Trim()
        if ($Settings -notcontains $name -or [Environment]::GetEnvironmentVariable($name, 'Process')) { continue }
        if ($value.Length -ge 2 -and (
            ($value.StartsWith('"') -and $value.EndsWith('"')) -or
            ($value.StartsWith("'") -and $value.EndsWith("'")))) {
            $value = $value.Substring(1, $value.Length - 2)
        }
        # Literal dotenv values only. No evaluation or interpolation.
        [Environment]::SetEnvironmentVariable($name, $value, 'Process')
    }
}
function Set-Default([string]$Name, [string]$Value) {
    if (-not [Environment]::GetEnvironmentVariable($Name, 'Process')) {
        [Environment]::SetEnvironmentVariable($Name, $Value, 'Process')
    }
}
function Require-Secret([string]$Name, [int]$MinimumBytes) {
    $value = [Environment]::GetEnvironmentVariable($Name, 'Process')
    if (-not $value) {
        $secure = Read-Host "$Name (reutilize o segredo existente; minimo $MinimumBytes bytes)" -AsSecureString
        $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
        try { $value = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer) }
        finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer) }
    }
    if ([Text.Encoding]::UTF8.GetByteCount($value) -lt $MinimumBytes) {
        throw "$Name ausente ou curto demais. Nenhum valor foi exibido."
    }
    [Environment]::SetEnvironmentVariable($Name, $value, 'Process')
}
function Get-BrokerQueues {
    $uri = [Uri]$env:RABBITMQ_URL
    $parts = $uri.UserInfo -split ':', 2
    if ($parts.Count -ne 2) { throw 'RABBITMQ_URL precisa informar usuario e senha locais.' }
    $credentials = [Uri]::UnescapeDataString($parts[0]) + ':' + [Uri]::UnescapeDataString($parts[1])
    $headers = @{ Authorization = 'Basic ' + [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($credentials)) }
    $vhost = [Uri]::UnescapeDataString($uri.AbsolutePath.TrimStart('/'))
    if (-not $vhost) { $vhost = '/' }
    $vhost = [Uri]::EscapeDataString($vhost)
    $response = Invoke-RestMethod -Uri "http://127.0.0.1:15672/api/queues/$vhost" -Headers $headers -TimeoutSec 3
    foreach ($queue in $response) { Write-Output $queue }
}
function Start-Terminal([string]$Name) {
    $arguments = @('-NoProfile','-NoExit','-ExecutionPolicy','Bypass','-File',
        ('"{0}"' -f $ScriptFile),'-Servico',$Name,'-Categoria',$Categoria,'-Intervalo',$Intervalo)
    # Visible windows require the explicit -AbrirTerminais switch.
    $process = Start-Process -FilePath (Get-Command powershell.exe).Source -ArgumentList $arguments -WorkingDirectory $ProjectRoot -WindowStyle Normal -PassThru
    Write-Host ("Aberto: {0} (terminal PID {1})" -f $Name, $process.Id)
}

Set-Location -LiteralPath $ProjectRoot
if ($Servico) {
    if ($Verificar) { throw 'Nao combine a opcao interna -Servico com -Verificar.' }
    if ($WhatIfPreference) { Write-Host "Simulacao do terminal: $Servico"; return }
    # Keep app credentials out of unrelated Go/npm subprocesses.
    $allowed = switch ($Servico) {
        'Estoque' { @('RABBITMQ_URL','DATABASE_URL','STOCK_PORT') }
        'Mock' { @('PAYMENT_WEBHOOK_SECRET','MOCK_PAYMENT_PORT','MOCK_PAYMENT_PUBLIC_URL','MOCK_CHECKOUT_TTL') }
        'Pagamento' { @('RABBITMQ_URL','DATABASE_URL','PAYMENT_WEBHOOK_SECRET','PAYMENT_PORT','PAYMENT_WEBHOOK_URL','MOCK_PAYMENT_URL') }
        'Entrega' { @('RABBITMQ_URL','DATABASE_URL') }
        'Interesses' { @('RABBITMQ_URL','DATABASE_URL') }
        'Ofertas' { @('RABBITMQ_URL','DATABASE_URL','RESEND_FROM','RESEND_TEST_RECIPIENT') }
        'Emails' { @('DATABASE_URL','RESEND_API_KEY','RESEND_FROM','RESEND_TEST_RECIPIENT') }
        'Gateway' { @('RABBITMQ_URL','DATABASE_URL','JWT_SECRET','STOCK_SERVICE_URL','FRONTEND_ORIGIN','GATEWAY_PORT') }
        'Publicador' { @('RABBITMQ_URL') }
        default { @() }
    }
    foreach ($name in $Settings) {
        if ($allowed -notcontains $name) { [Environment]::SetEnvironmentVariable($name, $null, 'Process') }
    }
    try { $Host.UI.RawUI.WindowTitle = "Event Market - $Servico" } catch { }
    Write-Host "Event Market - $Servico. Use Ctrl+C para parar."
    switch ($Servico) {
        'Estoque' { & go run ./cmd/estoque }
        'Mock' { & go run ./cmd/mock-pagamento }
        'Pagamento' { & go run ./cmd/pagamento }
        'Entrega' { & go run ./cmd/entrega }
        'Interesses' { & go run ./cmd/promocoes -interests }
        'Ofertas' { & go run ./cmd/promocoes -offers }
        'Emails' { & go run ./cmd/promocoes -emails }
        'Gateway' { & go run ./cmd/gateway }
        'Frontend' {
            Set-Location -LiteralPath (Join-Path $ProjectRoot 'frontend')
            & npm.cmd run dev -- --host localhost --port 5173 --strictPort
        }
        'Publicador' { & go run ./cmd/publish-promotions -interval $Intervalo -category $Categoria }
    }
    if ($LASTEXITCODE -ne 0) { Write-Warning "Programa encerrado com codigo $LASTEXITCODE. Confira o erro acima." }
    return
}

$services = @('Estoque','Mock','Pagamento','Entrega','Interesses','Ofertas','Gateway','Frontend')
if ($ComEmails) { $services += 'Emails' }
if ($PublicarOfertas) { $services += 'Publicador' }
Write-Host ("Plano: " + ($services -join ', '))
Write-Host 'PostgreSQL: Docker. RabbitMQ: servico nativo do Windows.'
if ($ComEmails) { Write-Warning 'Envios reais habilitados, inclusive jobs antigos pendentes.' }
if ($PublicarOfertas) { Write-Warning "Ofertas automaticas: $Categoria a cada $Intervalo." }
if ($WhatIfPreference -or (-not $Verificar -and -not $AbrirTerminais)) {
    Write-Host 'Use -Verificar para checar sem alterar; -AbrirTerminais para iniciar.'
    return
}
foreach ($tool in @('powershell.exe','go.exe','node.exe','npm.cmd','docker.exe')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) { throw "Comando necessario nao encontrado: $tool" }
}
foreach ($name in @('ms-principal','ms-estoque','ms-pagamento','ms-entrega','ms-promocoes')) {
    foreach ($file in @('private.pem','public.pem')) {
        if (-not (Test-Path -LiteralPath (Join-Path $ProjectRoot "keys/$name/$file"))) {
            throw "Chave ausente: keys/$name/$file. O script nao gera nem substitui chaves."
        }
    }
}
if (-not (Test-Path -LiteralPath (Join-Path $ProjectRoot 'frontend/node_modules/.bin/vite.cmd'))) {
    throw 'Execute npm ci na pasta frontend para instalar as dependencias.'
}
if ($Verificar) {
    Write-Host 'Somente leitura; nenhum segredo sera solicitado ou exibido.'
    Get-Service -Name RabbitMQ | Format-Table Name,Status
    & docker compose ps postgres rabbitmq
    foreach ($port in @(5432,5672,15672,8080,8081,8082,8083,5173)) {
        Write-Host ("Porta {0}: {1}" -f $port, $(if (Test-Port $port) { 'em uso' } else { 'livre' }))
    }
    return
}
if (-not $PSCmdlet.ShouldProcess('ambiente local','iniciar infraestrutura e abrir terminais')) { return }
$busy = @(8080,8081,8082,8083,5173 | Where-Object { Test-Port $_ })
$headless = @(Get-Process -Name entrega,promocoes,publish-promotions -ErrorAction SilentlyContinue)
if ($busy.Count -gt 0 -or $headless.Count -gt 0) {
    throw ("Programas ja abertos. Portas: " + ($busy -join ', ') + ". Use Ctrl+C e feche os terminais anteriores. Nada foi encerrado.")
}
$previous = @{}
foreach ($name in $Settings) { $previous[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    Import-LocalEnvironment
    Set-Default 'RABBITMQ_URL' 'amqp://guest:guest@127.0.0.1:5672/'
    Set-Default 'DATABASE_URL' 'postgres://ecommerce:ecommerce@localhost:5432/ecommerce?sslmode=disable'
    Set-Default 'RESEND_FROM' 'Event Market <onboarding@resend.dev>'
    if ($PSBoundParameters.ContainsKey('ValidadeCheckout')) { $env:MOCK_CHECKOUT_TTL = $ValidadeCheckout }
    Set-Default 'MOCK_CHECKOUT_TTL' $ValidadeCheckout
    foreach ($pair in @(
        @('STOCK_SERVICE_URL','http://localhost:8081'), @('FRONTEND_ORIGIN','http://localhost:5173'),
        @('GATEWAY_PORT','8080'), @('STOCK_PORT','8081'), @('PAYMENT_PORT','8082'),
        @('PAYMENT_WEBHOOK_URL','http://localhost:8082/webhooks/payments'),
        @('MOCK_PAYMENT_PORT','8083'), @('MOCK_PAYMENT_URL','http://localhost:8083'),
        @('MOCK_PAYMENT_PUBLIC_URL','http://localhost:8083')
    )) {
        Set-Default $pair[0] $pair[1]
        if ([Environment]::GetEnvironmentVariable($pair[0], 'Process') -ne $pair[1]) {
            throw "Este inicializador usa portas locais padrao. Configuracao diferente em $($pair[0]); ajuste ou inicie manualmente."
        }
    }
    try { $database = [Uri]$env:DATABASE_URL; $broker = [Uri]$env:RABBITMQ_URL } catch { throw 'DATABASE_URL ou RABBITMQ_URL invalida.' }
    if ($database.Scheme -notin @('postgres','postgresql') -or $database.Host -notin @('localhost','127.0.0.1') -or
        $database.Port -ne 5432 -or $database.AbsolutePath -ne '/ecommerce' -or $broker.Scheme -ne 'amqp' -or
        $broker.Host -notin @('localhost','127.0.0.1') -or $broker.Port -ne 5672) {
        throw 'Use somente ecommerce local em 5432 e RabbitMQ nativo local em 5672.'
    }
    Require-Secret 'JWT_SECRET' 32
    Require-Secret 'PAYMENT_WEBHOOK_SECRET' 32
    if ([string]::IsNullOrWhiteSpace($env:RESEND_TEST_RECIPIENT)) {
        $env:RESEND_TEST_RECIPIENT = (Read-Host 'E-mail destinatario de teste das ofertas').Trim()
    }
    try { $address = New-Object System.Net.Mail.MailAddress($env:RESEND_TEST_RECIPIENT) }
    catch { throw 'RESEND_TEST_RECIPIENT invalido.' }
    if ($address.Address -ne $env:RESEND_TEST_RECIPIENT) { throw 'Informe somente o endereco de e-mail de teste.' }
    if ($ComEmails) { Require-Secret 'RESEND_API_KEY' 1 }
    & docker info --format '{{.ServerVersion}}' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Abra o Docker Desktop e aguarde ele ficar pronto.' }
    $container = & docker compose ps --status running -q rabbitmq
    if ($LASTEXITCODE -ne 0) { throw 'Falha ao consultar RabbitMQ do Compose.' }
    if ($container) {
        Write-Host 'Parando somente RabbitMQ do Compose; sem apagar volumes.'
        & docker compose stop rabbitmq
        if ($LASTEXITCODE -ne 0) { throw 'Nao foi possivel parar o broker duplicado.' }
    }
    if ((Get-Service -Name RabbitMQ).Status -ne 'Running') {
        try { Start-Service -Name RabbitMQ }
        catch { throw 'Inicie o servico RabbitMQ como administrador e execute novamente. Nao ha elevacao automatica.' }
    }
    for ($i = 0; $i -lt 30 -and -not (Test-Port 5672); $i++) { Start-Sleep -Seconds 1 }
    if (-not (Test-Port 5672)) { throw 'RabbitMQ nativo inacessivel na porta 5672.' }
    & docker compose up -d postgres
    if ($LASTEXITCODE -ne 0) { throw 'Falha ao iniciar PostgreSQL.' }
    $dbUser = [Uri]::UnescapeDataString(($database.UserInfo -split ':',2)[0])
    if (-not $dbUser) { throw 'DATABASE_URL precisa informar o usuario do banco.' }
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        & docker compose exec -T postgres pg_isready -U $dbUser -d ecommerce *> $null
        if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        Start-Sleep -Seconds 1
    }
    if (-not $ready) { throw 'PostgreSQL nao ficou pronto.' }
    $query = "SELECT CASE WHEN to_regclass('gateway.event_outbox') IS NOT NULL AND to_regclass('gateway.promotion_outbox') IS NOT NULL AND to_regclass('promocoes.email_jobs') IS NOT NULL AND (SELECT count(*) FROM information_schema.columns WHERE (table_schema='estoque' AND table_name='stock_reservations' AND column_name='request_payload') OR (table_schema='entrega' AND table_name='shipments' AND column_name='payment_payload')) = 2 THEN 'OK' ELSE 'MIGRATIONS_PENDENTES' END;"
    $schema = & docker compose exec -T postgres psql -U $dbUser -d ecommerce -tA -v ON_ERROR_STOP=1 -c $query
    if ($LASTEXITCODE -ne 0 -or ($schema -join '').Trim() -ne 'OK') {
        throw 'Aplique as migrations 001-016 necessarias antes de iniciar. O script nao altera o schema.'
    }
    foreach ($name in $services) {
        Start-Terminal $name
        switch ($name) {
            'Estoque' { Wait-Health 8081 }
            'Mock' { Wait-Health 8083 }
            'Pagamento' { Wait-Health 8082 }
            'Gateway' { Wait-Health 8080 }
            'Frontend' { Wait-Health 5173 }
        }
    }
    $expected = @('estoque','pagamento','entrega','gateway.pedidos.status','promocoes.interesses','promocoes.ofertas')
    $missing = $expected
    $deadline = (Get-Date).AddSeconds(45)
    do {
        $queues = @(Get-BrokerQueues)
        $missing = @($expected | Where-Object { $wanted = $_; -not ($queues | Where-Object { $_.name -eq $wanted -and $_.consumers -gt 0 }) })
        if ($missing.Count -eq 0) { break }
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $deadline)
    if ($missing.Count -gt 0) { throw ("Filas sem consumidor: " + ($missing -join ', ') + ". Confira os terminais.") }
    Write-Host 'Pronto: http://localhost:5173 | RabbitMQ: http://127.0.0.1:15672'
    Write-Host 'Para parar: Ctrl+C nos terminais. Nao reinicie o Mock durante um checkout.'
    if (-not $ComEmails) { Write-Host 'Envio de e-mails DESLIGADO. -ComEmails autoriza envios reais.' }
    if (-not $PublicarOfertas) { Write-Host 'Ofertas automaticas DESLIGADAS. Use -PublicarOfertas para habilitar.' }
} finally {
    foreach ($name in $Settings) { [Environment]::SetEnvironmentVariable($name, $previous[$name], 'Process') }
}
