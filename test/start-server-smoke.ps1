#Requires -Version 7.0
# 实际启动临时 Docker 服务，覆盖首次引导、重复运行及停止后重建。
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$launcher = Join-Path $repoRoot 'start-server.ps1'
$name = 'ginx-launch-test-' + [Guid]::NewGuid().ToString('N').Substring(0, 8)
$tempDir = Join-Path ([IO.Path]::GetTempPath()) "$name with spaces"
New-Item -ItemType Directory -Path $tempDir | Out-Null
$accountFile = Join-Path $tempDir 'accounts.json'
$playersFile = Join-Path $tempDir 'players.db'
$configFile = Join-Path $repoRoot 'config/ginx.json'
$originalConfigHash = (Get-FileHash -LiteralPath $configFile).Hash
$testPassword = [Guid]::NewGuid().ToString('N')
$promptState = @{ Count = 0 }

# 测试只替换交互输入，真实调用构建、密码哈希、容器启动和健康检查。
function Read-Host {
    param([string]$Prompt, [switch]$AsSecureString)
    $promptState.Count++
    if ($AsSecureString) { return ConvertTo-SecureString $testPassword -AsPlainText -Force }
    return 'launcher-test'
}

function Assert-True {
    param([bool]$Condition, [string]$Message)
    if (-not $Condition) { throw $Message }
}

$httpListener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$tcpListener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, 0)
$httpListener.Start()
$tcpListener.Start()
$httpPort = $httpListener.LocalEndpoint.Port
$tcpPort = $tcpListener.LocalEndpoint.Port
$httpListener.Stop()
$tcpListener.Stop()
$launchParams = @{ HttpPort = $httpPort; TcpPort = $tcpPort; ContainerName = $name; AccountsPath = $accountFile; PlayersPath = $playersFile }

try {
    # 任意 cwd、含空格路径和首次账号初始化。
    Push-Location $tempDir
    try { & $launcher @launchParams }
    finally { Pop-Location }
    Assert-True ($promptState.Count -eq 3) 'First launch did not ask for the account and two password entries.'
    $savedAccounts = Get-Content -LiteralPath $accountFile -Raw
    Assert-True (-not $savedAccounts.Contains($testPassword)) 'Plaintext password was stored.'
    $accountHash = (Get-FileHash -LiteralPath $accountFile).Hash
    $loginBody = @{ account_id = 'launcher-test'; password = $testPassword } | ConvertTo-Json
    $login = Invoke-RestMethod -Uri "http://127.0.0.1:$httpPort/api/v1/login" -Method Post -ContentType application/json -Body $loginBody -NoProxy -TimeoutSec 5
    Assert-True ($login.player_id -eq 1001 -and $login.token.Length -gt 0) 'Created account cannot log in.'
    Assert-True (Test-Path -LiteralPath $playersFile) 'Player storage was not created in the requested location.'
    # 真正运行示例客户端：登录、鉴权、入房、领奖、消耗和查询。
    $clientArgs = @('run', '--rm', '-i', '--volume', "${repoRoot}:/workspace", '--workdir', '/workspace',
        '--volume', 'gin-go-mod:/go/pkg/mod', '--volume', 'gin-go-build:/root/.cache/go-build',
        'golang:1.26-bookworm', 'go', 'run', './examples/gameclient', '-account', 'launcher-test',
        '-http', "http://host.docker.internal:$httpPort", '-tcp', "host.docker.internal:$tcpPort")
    $clientOutput = $testPassword | & docker @clientArgs
    Assert-True ($LASTEXITCODE -eq 0) 'Example client failed.'
    Assert-True (($clientOutput | Out-String) -match '"potion":1') 'Example client did not persist progression.'
    $login = Invoke-RestMethod -Uri "http://127.0.0.1:$httpPort/api/v1/login" -Method Post -ContentType application/json -Body $loginBody -NoProxy -TimeoutSec 5
    $before = Invoke-RestMethod -Uri "http://127.0.0.1:$httpPort/api/v1/progress" -Headers @{ Authorization = "Bearer $($login.token)" } -NoProxy -TimeoutSec 5
    Assert-True ($before.level -eq 2 -and $before.inventory.potion -eq 1 -and $before.starter_claimed) 'Progress did not match the client actions.'
    $firstID = docker inspect --format '{{.Id}}' $name
    Assert-True ($LASTEXITCODE -eq 0) 'First container was not found.'

    & $launcher @launchParams -NonInteractive
    $secondID = docker inspect --format '{{.Id}}' $name
    Assert-True ($firstID -eq $secondID) 'Repeated launch replaced the running container.'
    docker stop $name | Out-Null
    Assert-True ($LASTEXITCODE -eq 0) 'Could not stop the test container.'
    & $launcher @launchParams -NonInteractive
    $thirdID = docker inspect --format '{{.Id}}' $name
    Assert-True ($firstID -ne $thirdID) 'Stopped container was not rebuilt/replaced.'
    Assert-True ((Get-FileHash -LiteralPath $accountFile).Hash -eq $accountHash) 'Existing account file changed.'
    $login = Invoke-RestMethod -Uri "http://127.0.0.1:$httpPort/api/v1/login" -Method Post -ContentType application/json -Body $loginBody -NoProxy -TimeoutSec 5
    $after = Invoke-RestMethod -Uri "http://127.0.0.1:$httpPort/api/v1/progress" -Headers @{ Authorization = "Bearer $($login.token)" } -NoProxy -TimeoutSec 5
    Assert-True (($before | ConvertTo-Json -Compress) -eq ($after | ConvertTo-Json -Compress)) 'Existing progress changed after restart.'
    Assert-True ((Get-FileHash -LiteralPath $configFile).Hash -eq $originalConfigHash) 'Repository TCP configuration changed.'
    Assert-True ($promptState.Count -eq 3) 'Existing account prompted for credentials again.'

    # 端口被占用时应退出非零并清理本次新容器，不污染成功实例。
    $failedName = "$name-invalid"
    $failureOutput = & pwsh -NoLogo -NoProfile -File $launcher -HttpPort $httpPort -TcpPort $tcpPort -ContainerName $failedName -AccountsPath $accountFile -PlayersPath (Join-Path $tempDir 'invalid-players.json') -NonInteractive 2>&1
    Assert-True ($LASTEXITCODE -ne 0) 'A failed launch reported success.'
    Assert-True (($failureOutput | Out-String) -match "Docker 'start' failed") 'Expected a Docker startup/port conflict failure.'
    $leftover = docker ps -aq --filter "name=^/$failedName$"
    Assert-True ([string]::IsNullOrEmpty($leftover)) 'A failed launch left its container behind.'
    $stillRunning = docker inspect --format '{{.State.Running}}' $name
    Assert-True ($stillRunning -eq 'true') 'Failed launch affected the existing server.'
    $missingFile = Join-Path $tempDir 'missing.json'
    $missingOutput = & pwsh -NoLogo -NoProfile -File $launcher -ContainerName $failedName -AccountsPath $missingFile -NonInteractive 2>&1
    Assert-True ($LASTEXITCODE -ne 0 -and ($missingOutput | Out-String) -match 'Account file is missing') 'Noninteractive startup did not reject missing credentials.'
    Assert-True (-not (Test-Path -LiteralPath $missingFile)) 'Noninteractive startup generated credentials.'
    Write-Host 'PASS: first launch, real HTTP/TCP client, SQLite progression, paths with spaces, reuse, rebuild, persistence and failure cleanup.'
}
finally {
    docker rm --force $name 2>$null | Out-Null
    docker rm --force "$name-invalid" 2>$null | Out-Null
    # 保留少量忽略目录中的构建产物；测试账号只在系统临时目录中。
    $resolvedTemp = [IO.Path]::GetFullPath($tempDir)
    $expectedTemp = Join-Path ([IO.Path]::GetTempPath()) "$name with spaces"
    if ($resolvedTemp -eq [IO.Path]::GetFullPath($expectedTemp) -and (Split-Path -Leaf $resolvedTemp) -eq "$name with spaces") {
        Remove-Item -LiteralPath $resolvedTemp -Recurse -Force
    }
}
