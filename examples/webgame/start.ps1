#Requires -Version 7.0
param(
    [ValidateRange(1, 65535)][int]$Port = 8090,
    [switch]$Stop
)
$ErrorActionPreference = 'Stop'
$composePath = Join-Path $PSScriptRoot 'compose.yaml'
if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Docker is required. Start Docker Desktop in Linux container mode.' }
$priorPort = $env:GINX_WEB_PORT
try {
    $env:GINX_WEB_PORT = [string]$Port
    $composeArgs = @('compose', '-p', 'ginx-webgame', '-f', $composePath)
    if ($Stop) {
        & docker @composeArgs down
        if ($LASTEXITCODE -ne 0) { throw 'Could not stop the webgame stack.' }
        Write-Host 'Ginx webgame stopped.'
        return
    }
    & docker @composeArgs up -d --build --wait --wait-timeout 90
    if ($LASTEXITCODE -ne 0) { throw 'Webgame build/start failed. Inspect: docker compose -p ginx-webgame -f examples/webgame/compose.yaml logs' }
    $health = Invoke-RestMethod -Uri "http://127.0.0.1:$Port/healthz" -TimeoutSec 5
    if ($health.status -ne 'ok') { throw 'Webgame health check failed.' }
    Write-Host "Ginx Arena ready: http://127.0.0.1:$Port"
    Write-Host 'Open two browser tabs, choose different names, and join the same room.'
    Write-Host 'Stop: pwsh -File examples/webgame/start.ps1 -Stop'
}
finally {
    $env:GINX_WEB_PORT = $priorPort
}
