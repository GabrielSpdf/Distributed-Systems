# Local tests with mocked process/network/file readers. No services are started.
$ErrorActionPreference = 'Stop'
$initialLocation = Get-Location
. (Join-Path $PSScriptRoot 'start-dev.ps1') -WhatIf

function Assert([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw $Message }
}
$savedSettings = @{}
foreach ($name in $Settings) { $savedSettings[$name] = [Environment]::GetEnvironmentVariable($name, 'Process') }
try {
    $fixtureLines = @(
        '# synthetic fixture, not a real dotenv file',
        'JWT_SECRET="literal-$(not-a-command)"',
        'DATABASE_URL="fixture-value"',
        'export FRONTEND_ORIGIN=''http://localhost:5173''',
        'UNRELATED_SETTING=not-imported'
    )
    $unrelatedBefore = [Environment]::GetEnvironmentVariable('UNRELATED_SETTING', 'Process')
    [Environment]::SetEnvironmentVariable('JWT_SECRET', $null, 'Process')
    [Environment]::SetEnvironmentVariable('DATABASE_URL', 'preferred-existing-value', 'Process')
    [Environment]::SetEnvironmentVariable('FRONTEND_ORIGIN', $null, 'Process')
    function Test-Path { param($LiteralPath) return $true }
    function Get-Content { param($LiteralPath, $Encoding) return $fixtureLines }
    Import-LocalEnvironment
    Assert ($env:JWT_SECRET -eq 'literal-$(not-a-command)') 'dotenv executed or altered literal content'
    Assert ($env:DATABASE_URL -eq 'preferred-existing-value') 'dotenv overwrote the current environment'
    Assert ($env:FRONTEND_ORIGIN -eq 'http://localhost:5173') 'quoted dotenv value was not read'
    Assert ([Environment]::GetEnvironmentVariable('UNRELATED_SETTING', 'Process') -eq $unrelatedBefore) 'unknown setting imported'
    Remove-Item -LiteralPath Function:\Test-Path,Function:\Get-Content
    Write-Host 'PASS: literal dotenv, allowed settings, environment precedence'

    $env:JWT_SECRET = 'x' * 32
    Require-Secret 'JWT_SECRET' 32
    $env:JWT_SECRET = 'x' * 31
    $rejected = $false
    try { Require-Secret 'JWT_SECRET' 32 } catch { $rejected = $true }
    Assert $rejected 'short secret was accepted'
    Write-Host 'PASS: minimum secret size without interactive prompts'

    $script:launch = $null
    function Start-Process {
        param($FilePath, $ArgumentList, $WorkingDirectory, $WindowStyle, [switch]$PassThru)
        $script:launch = @{ File=$FilePath; Arguments=$ArgumentList; Directory=$WorkingDirectory; Style=$WindowStyle }
        return [pscustomobject]@{ Id = 123 }
    }
    Start-Terminal 'Gateway'
    Assert ($launch.Directory -eq $ProjectRoot) 'wrong working directory'
    $fileIndex = [Array]::IndexOf($launch.Arguments, '-File')
    Assert ($fileIndex -ge 0 -and $launch.Arguments[$fileIndex + 1] -eq ('"{0}"' -f $ScriptFile)) 'script path with spaces not quoted'
    Assert (-not (($launch.Arguments -join ' ').Contains($env:JWT_SECRET))) 'secret exposed in process arguments'
    Assert ($launch.Style -eq 'Normal') 'requested terminal is not visible'
    Remove-Item -LiteralPath Function:\Start-Process
    Write-Host 'PASS: quoted terminal path, no credentials in process arguments'

    $env:RABBITMQ_URL = 'amqp://unit:unit@127.0.0.1:5672/%2F'
    $script:requestedUri = ''
    function Invoke-RestMethod {
        param($Uri, $Headers, $TimeoutSec)
        $script:requestedUri = $Uri
        return ,@(
            [pscustomobject]@{ name = 'estoque'; consumers = 1 },
            [pscustomobject]@{ name = 'gateway.promocoes.exemplo'; consumers = 0 }
        )
    }
    $queues = @(Get-BrokerQueues)
    Assert ($requestedUri -eq 'http://127.0.0.1:15672/api/queues/%2F') 'wrong vhost URL'
    Assert ($queues.Count -eq 2) 'management response not flattened'
    Assert (@($queues | Where-Object { $_.name -eq 'gateway.promocoes.exemplo' -and $_.consumers -gt 0 }).Count -eq 0) 'inactive queue treated as active'
    Remove-Item -LiteralPath Function:\Invoke-RestMethod
    Write-Host 'PASS: native management endpoint and encoded vhost'
} finally {
    foreach ($name in $Settings) { [Environment]::SetEnvironmentVariable($name, $savedSettings[$name], 'Process') }
    Set-Location -LiteralPath $initialLocation
}
Write-Host 'All startup tests passed. No terminals opened; no real dotenv file read.'
