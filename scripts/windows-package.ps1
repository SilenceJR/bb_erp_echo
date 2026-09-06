param(
  [Parameter(Mandatory = $true)][ValidatePattern('^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$')][string]$Version,
  [string]$OutputDir = 'release-build'
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$outputRoot = if ([IO.Path]::IsPathRooted($OutputDir)) { $OutputDir } else { Join-Path $repoRoot $OutputDir }
$stageRoot = Join-Path $outputRoot 'packages'
$serverStage = Join-Path $stageRoot 'server'
$clientUpdateStage = Join-Path $stageRoot 'client-update'
$allInOneStage = Join-Path $stageRoot 'all-in-one'

function Require-File([string]$Path, [string]$Message) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { throw $Message }
}
function Write-Utf8NoBom([string]$Path, [string]$Value) {
  [IO.File]::WriteAllText($Path, $Value, [Text.UTF8Encoding]::new($false))
}
function Compress-Directory([string]$Source, [string]$Destination) {
  if (Test-Path -LiteralPath $Destination) { Remove-Item -LiteralPath $Destination -Force }
  Compress-Archive -Path (Join-Path $Source '*') -DestinationPath $Destination -CompressionLevel Optimal
}

$publicKey = (& (Join-Path $PSScriptRoot 'normalize-tauri-public-key.ps1') -Value $env:TAURI_UPDATER_PUBLIC_KEY) -join ''
$signingPrivateKey = $env:TAURI_SIGNING_PRIVATE_KEY
$signingPassword = $env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD
if (-not $signingPrivateKey -or -not $signingPassword -or -not $publicKey) {
  throw 'TAURI_SIGNING_PRIVATE_KEY、TAURI_SIGNING_PRIVATE_KEY_PASSWORD 和 TAURI_UPDATER_PUBLIC_KEY 必须配置。'
}

# Dependency installation and application builds must never inherit signing
# secrets. Restore them only for the narrow package-signing subprocess below.
Remove-Item Env:TAURI_SIGNING_PRIVATE_KEY -ErrorAction SilentlyContinue
Remove-Item Env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD -ErrorAction SilentlyContinue

Remove-Item -LiteralPath $stageRoot -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force -Path $outputRoot, $serverStage, $clientUpdateStage, $allInOneStage | Out-Null

Push-Location $repoRoot
try {
  Push-Location web
  try { npm ci; npm run build } finally { Pop-Location }
  $env:CGO_ENABLED = '1'
  go build -tags nodynamic -trimpath -ldflags "-s -w -X bb_erp_echo/internal/buildinfo.Version=$Version" -o (Join-Path $outputRoot 'bb-erp-server.exe') ./cmd/server
  $tauriConfigPath = Join-Path $outputRoot 'tauri-version.json'
  Write-Utf8NoBom $tauriConfigPath (@{version=$Version;bundle=@{active=$false}} | ConvertTo-Json -Compress)
  $env:BB_ERP_UPDATE_PUBLIC_KEY = $publicKey
  Push-Location client
  try { npm ci; npm run desktop:build -- --no-bundle --config $tauriConfigPath } finally { Pop-Location }
} finally { Pop-Location }

$serverExe = Join-Path $outputRoot 'bb-erp-server.exe'
$clientExe = Join-Path $repoRoot 'client/src-tauri/target/release/bb_erp_client.exe'
Require-File $serverExe '未生成服务端 EXE。'
Require-File $clientExe '未生成客户端 EXE。'

New-Item -ItemType Directory -Force -Path (Join-Path $serverStage 'web'), (Join-Path $serverStage 'data'), (Join-Path $serverStage 'logs'), (Join-Path $serverStage 'static/uploads'), (Join-Path $serverStage 'updates/client-cache') | Out-Null
Copy-Item -LiteralPath $serverExe -Destination (Join-Path $serverStage 'bb-erp-server.exe')
Copy-Item -LiteralPath (Join-Path $repoRoot 'web/dist') -Destination (Join-Path $serverStage 'web/dist') -Recurse
Write-Utf8NoBom (Join-Path $serverStage 'update-public.key') $publicKey
Write-Utf8NoBom (Join-Path $serverStage 'version.json') (([ordered]@{version=$Version;server_version=$Version}) | ConvertTo-Json -Compress)
Set-Content -LiteralPath (Join-Path $serverStage '启动服务端.bat') -Encoding ASCII -Value @'
@echo off
cd /d "%~dp0"
set BB_ERP_APP_ENVIRONMENT=production
set BB_ERP_HTTP_HOST=0.0.0.0
set BB_ERP_HTTP_PORT=8080
set BB_ERP_DATABASE_PATH=data\erp.db
set BB_ERP_LOG_DIR=logs
set BB_ERP_FILES_ROOT_DIR=static\uploads
set BB_ERP_WEB_ENABLED=true
set BB_ERP_WEB_DIST_DIR=web\dist
set BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key
bb-erp-server.exe
pause
'@

$env:RELEASE_CLIENT_EXE = $clientExe
$env:RELEASE_OUTPUT_DIR = $clientUpdateStage
$env:RELEASE_VERSION = $Version
$env:TAURI_UPDATER_PUBLIC_KEY = $publicKey
$env:TAURI_SIGNING_PRIVATE_KEY = $signingPrivateKey
$env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD = $signingPassword
bash (Join-Path $PSScriptRoot 'build-client-update.sh')
Remove-Item Env:TAURI_SIGNING_PRIVATE_KEY -ErrorAction SilentlyContinue
Remove-Item Env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD -ErrorAction SilentlyContinue

Copy-Item -LiteralPath $serverStage -Destination (Join-Path $allInOneStage 'server') -Recurse
Copy-Item -LiteralPath $clientUpdateStage -Destination (Join-Path $allInOneStage 'client') -Recurse
Set-Content -LiteralPath (Join-Path $allInOneStage '启动系统.bat') -Encoding ASCII -Value @'
@echo off
start "BB ERP Server" /d "%~dp0server" "%~dp0server\启动服务端.bat"
timeout /t 3 /nobreak >nul
start "BB ERP Client" "%~dp0client\bb_erp_client.exe"
'@
Write-Utf8NoBom (Join-Path $allInOneStage 'README.txt') "博邦 ERP Windows 完整包`r`n版本：$Version`r`n解压后运行 启动系统.bat。服务端数据目录升级时必须保留。"

Compress-Directory $serverStage (Join-Path $outputRoot "bb-erp-server-windows-v$Version.zip")
Compress-Directory $clientUpdateStage (Join-Path $outputRoot "bb-erp-client-update-windows-v$Version.zip")
Compress-Directory $allInOneStage (Join-Path $outputRoot "bb-erp-all-in-one-windows-v$Version.zip")
Write-Host "Windows 离线产物已生成：$outputRoot"
