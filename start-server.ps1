#Requires -Version 7.0
<#
.SYNOPSIS
Build and start the Gin HTTP + Ginx TCP server with Docker.
.EXAMPLE
./start-server.ps1
.EXAMPLE
./start-server.ps1 -HttpPort 18080 -TcpPort 17777 -NonInteractive
#>
[CmdletBinding()]
param(
    [ValidateRange(1, 65535)][int]$HttpPort = 8080,
    [ValidateRange(0, 65535)][int]$TcpPort = 0,
    [ValidatePattern('^[a-zA-Z0-9][a-zA-Z0-9_.-]*$')][string]$ContainerName = 'ginx-game',
    [string]$AccountsPath = 'config/accounts.local.json',
    [ValidateSet('sqlite', 'json')][string]$Storage = 'sqlite',
    [string]$PlayersPath = '',
    [string]$DockerImage = 'golang:1.26-bookworm',
    [switch]$NonInteractive
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
$repoRoot = $PSScriptRoot
$createdID = $null
$launchLock = $null

function Invoke-Docker {
    param([string[]]$Arguments)
    & docker @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Docker '$($Arguments[0])' failed (exit $LASTEXITCODE). See the output above."
    }
}

function Resolve-ProjectPath {
    param([string]$Path)
    if ([string]::IsNullOrWhiteSpace($Path)) { throw 'A file path cannot be empty.' }
    if (-not [IO.Path]::IsPathRooted($Path)) { $Path = Join-Path $repoRoot $Path }
    return [IO.Path]::GetFullPath($Path)
}

function Wait-Server {
    param([string]$ID)
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    do {
        $running = Invoke-Docker @('inspect', '--format', '{{.State.Running}}', $ID)
        if ($running -ne 'true') { throw 'The server container exited before becoming ready.' }
        $client = [Net.Sockets.TcpClient]::new()
        try {
            $health = Invoke-RestMethod -Uri "http://127.0.0.1:$HttpPort/healthz" -NoProxy -TimeoutSec 2
            if ($health.status -eq 'ok' -and $client.ConnectAsync('127.0.0.1', $TcpPort).Wait(1000)) {
                return
            }
        }
        catch { Write-Verbose $_.Exception.Message }
        finally { $client.Dispose() }
        Start-Sleep -Milliseconds 250
    } while ([DateTime]::UtcNow -lt $deadline)
    throw 'HTTP/TCP readiness checks did not pass within 30 seconds.'
}

function Show-Server {
    Write-Host "HTTP: http://127.0.0.1:$HttpPort"
    Write-Host "TCP:  127.0.0.1:$TcpPort"
    Write-Host "Logs: docker logs --follow $ContainerName"
    Write-Host "Stop: docker stop $ContainerName"
}

try {
    if (-not (Get-Command docker -CommandType Application -ErrorAction SilentlyContinue)) {
        throw 'Docker is not installed or not on PATH. Install/start Docker Desktop first.'
    }
    $osType = Invoke-Docker @('info', '--format', '{{.OSType}}')
    if ($osType -ne 'linux') { throw 'Docker must be running in Linux container mode.' }

    $config = Get-Content -LiteralPath (Join-Path $repoRoot 'config/ginx.json') -Raw | ConvertFrom-Json
    if ($TcpPort -eq 0) { $TcpPort = [int]$config.TcpPort }
    if ($TcpPort -lt 1 -or $TcpPort -gt 65535 -or $TcpPort -eq $HttpPort) {
        throw 'HTTP and TCP need distinct ports in the range 1..65535. Use -HttpPort / -TcpPort.'
    }
    if ($PlayersPath -eq '') {
        $PlayersPath = if ($Storage -eq 'sqlite') { 'data/players.db' } else { 'data/players.json' }
    }
    $accountsFile = Resolve-ProjectPath $AccountsPath
    $playersFile = Resolve-ProjectPath $PlayersPath
    if ($accountsFile -eq $playersFile) { throw 'Account configuration and player storage must use different files.' }
    $runtimeDir = Join-Path $repoRoot "data/runtime/$ContainerName"
    New-Item -ItemType Directory -Path $runtimeDir -Force | Out-Null
    # 同一入口串行启动，避免覆盖正在构建或启动的文件。
    $launchLock = [IO.File]::Open((Join-Path $runtimeDir 'start.lock'), 'OpenOrCreate', 'ReadWrite', 'None')

    $existingIDs = @(Invoke-Docker @('ps', '-aq', '--filter', "name=^/$ContainerName$"))
    if ($existingIDs.Count -gt 0) {
        $existing = (Invoke-Docker @('inspect', $existingIDs[0]) | ConvertFrom-Json)[0]
        $labels = $existing.Config.Labels
        if ($null -eq $labels -or $labels.'ginx.launcher.root' -ne $repoRoot) {
            throw "Container '$ContainerName' is not owned by this checkout. Choose another -ContainerName."
        }
        if ($existing.State.Running) {
            if ($labels.'ginx.launcher.http' -ne "$HttpPort" -or $labels.'ginx.launcher.tcp' -ne "$TcpPort") {
                throw "Container '$ContainerName' uses different ports. Stop it before changing ports."
            }
            Wait-Server $existing.Id
            Write-Host 'Server is already running; no rebuild or restart was performed.'
            Show-Server
            return
        }
    }

    $createAccount = -not (Test-Path -LiteralPath $accountsFile -PathType Leaf)
    if ($createAccount -and ($NonInteractive -or [Environment]::GetCommandLineArgs().Contains('-NonInteractive'))) {
        throw "Account file is missing: $accountsFile. Run start-server.cmd interactively once to create it."
    }
    if (-not $createAccount) {
        $accounts = @(Get-Content -LiteralPath $accountsFile -Raw | ConvertFrom-Json)
        if ($accounts.Count -eq 0) { throw 'Account configuration must contain at least one account.' }
    }

    New-Item -ItemType Directory -Path (Join-Path $runtimeDir 'config') -Force | Out-Null
    # 只改容器副本；保留仓库配置及已有账号、存档。
    $config.Host = '0.0.0.0'
    $config.TcpPort = $TcpPort
    $config | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $runtimeDir 'config/ginx.json') -Encoding utf8NoBOM
    New-Item -ItemType Directory -Path (Split-Path -Parent $playersFile) -Force | Out-Null

    Write-Host 'Building gameserver in Docker (first run may download Go modules)...'
    Invoke-Docker @(
        'run', '--rm', '--volume', "${repoRoot}:/workspace", '--workdir', '/workspace',
        '--volume', 'gin-go-mod:/go/pkg/mod', '--volume', 'gin-go-build:/root/.cache/go-build',
        $DockerImage, 'go', 'build', '-o', "data/runtime/$ContainerName/gameserver", './examples/gameserver'
    )

    if ($createAccount) {
        Write-Host 'First launch: create a local account (no default password). Player ID: 1001.'
        $accountID = (Read-Host 'Account ID [alice]').Trim()
        if ($accountID -eq '') { $accountID = 'alice' }
        if ([Text.Encoding]::UTF8.GetByteCount($accountID) -gt 128) { throw 'Account ID must be at most 128 UTF-8 bytes.' }
        $secret = Read-Host 'Password (8..72 UTF-8 bytes)' -AsSecureString
        $confirmation = Read-Host 'Confirm password' -AsSecureString
        try {
            $password = [Net.NetworkCredential]::new('', $secret).Password
            $confirmed = [Net.NetworkCredential]::new('', $confirmation).Password
            $length = [Text.Encoding]::UTF8.GetByteCount($password)
            if ($length -lt 8 -or $length -gt 72 -or $password.Contains("`n") -or $password.Contains("`r")) {
                throw 'Password must be 8..72 UTF-8 bytes and contain no line breaks.'
            }
            if ($password -cne $confirmed) { throw 'Passwords do not match. No account file was created.' }
            # 密码仅通过 stdin 传给已有哈希入口，不放入命令行、环境变量或日志。
            $hashArgs = @('run', '--rm', '-i', '--volume', "${runtimeDir}:/app:ro", '--workdir', '/app',
                $DockerImage, '/app/gameserver', '-hash-password')
            $hash = $password | & docker @hashArgs
            if ($LASTEXITCODE -ne 0 -or $hash -notmatch '^\$2[aby]\$\d{2}\$[./A-Za-z0-9]{53}$') {
                throw 'Password hashing failed; no account file was created.'
            }
        }
        finally {
            $password = $null
            $confirmed = $null
            $secret.Dispose()
            $confirmation.Dispose()
        }
        $accountJSON = ConvertTo-Json -InputObject @(@{ account_id = $accountID; player_id = 1001; password_hash = $hash })
        New-Item -ItemType Directory -Path (Split-Path -Parent $accountsFile) -Force | Out-Null
        $accountStream = [IO.File]::Open($accountsFile, 'CreateNew', 'Write', 'None')
        try {
            $bytes = [Text.Encoding]::UTF8.GetBytes($accountJSON)
            $accountStream.Write($bytes, 0, $bytes.Length)
        }
        finally { $accountStream.Dispose() }
        Write-Host "Created local account '$accountID'; only the bcrypt hash was saved."
    }

    if ($existingIDs.Count -gt 0) {
        # 仅替换已经验证归属且停止的旧容器，不删除挂载数据。
        Invoke-Docker @('rm', $existingIDs[0]) | Out-Null
    }
    $playersDir = Split-Path -Parent $playersFile
    $playersName = Split-Path -Leaf $playersFile
    $createdID = Invoke-Docker @(
        'create', '--name', $ContainerName, '--stop-timeout', '10',
        '--label', "ginx.launcher.root=$repoRoot", '--label', "ginx.launcher.http=$HttpPort", '--label', "ginx.launcher.tcp=$TcpPort",
        '--publish', "127.0.0.1:${HttpPort}:$HttpPort", '--publish', "127.0.0.1:${TcpPort}:$TcpPort",
        '--volume', "${runtimeDir}:/app:ro", '--volume', "${accountsFile}:/accounts.json:ro",
        '--volume', "${playersDir}:/data", '--workdir', '/app',
        $DockerImage, '/app/gameserver', '-http', "0.0.0.0:$HttpPort", '-accounts', '/accounts.json', '-storage', $Storage, '-players', "/data/$playersName"
    )
    Invoke-Docker @('start', $createdID) | Out-Null
    Wait-Server $createdID
    Write-Host 'Server is ready and running in the background. Closing this window will not stop it.'
    Show-Server
    $createdID = $null
}
catch {
    if ($createdID) {
        & docker logs --tail 40 $createdID
        & docker rm --force $createdID | Out-Null
    }
    Write-Error $_ -ErrorAction Continue
    exit 1
}
finally {
    if ($launchLock) { $launchLock.Dispose() }
}
