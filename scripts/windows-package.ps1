[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [string]$Version,

  [ValidateSet("AllInOne", "All")]
  [string]$Target = "AllInOne",

  [string]$OutputDir = "release-build"
)

# Windows 本机发布入口。它不会推送代码、创建标签、上传制品或修改服务器目录。
# 正式更新包仅在 -Target All 时生成；默认只生成全新安装用的 all-in-one 包。
Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Fail([string]$Message) { throw $Message }
function Require-Command([string]$Name) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) { Fail "缺少命令：$Name。请检查 Windows 构建环境和 PATH。" }
}
function Require-File([string]$Path, [string]$Message) {
  if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) { Fail $Message }
}
function Write-Utf8NoBom([string]$Path, [string]$Text) {
  [IO.File]::WriteAllText($Path, $Text, (New-Object Text.UTF8Encoding($false)))
}
function Get-EnvValueOrFile([string]$Name) {
  $value = [string][Environment]::GetEnvironmentVariable($Name)
  if ([string]::IsNullOrWhiteSpace($value)) { Fail "未配置环境变量 $Name。" }
  if (Test-Path -LiteralPath $value -PathType Leaf) { return (Get-Content -LiteralPath $value -Raw) }
  return $value
}
function Get-FileSha256([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash.ToLowerInvariant() }
function Copy-TreeContents([string]$Source, [string]$Destination) {
  New-Item -ItemType Directory -Force -Path $Destination | Out-Null
  Get-ChildItem -LiteralPath $Source -Force | Copy-Item -Destination $Destination -Recurse -Force
}
function Compress-Directory([string]$Source, [string]$Destination) {
  if (Test-Path -LiteralPath $Destination) { Remove-Item -LiteralPath $Destination -Force }
  Compress-Archive -Path (Join-Path $Source '*') -DestinationPath $Destination -Force
}
function Assert-LastExit([string]$Operation) {
  if ($LASTEXITCODE -ne 0) { Fail "$Operation 失败，退出码：$LASTEXITCODE" }
}

if ($Version -notmatch '^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$') { Fail "Version 必须是正式 MAJOR.MINOR.PATCH，例如 0.0.13。" }
if ($env:OS -ne 'Windows_NT') { Fail "windows-package.ps1 只能在 Windows x64 上运行。" }
if ([Environment]::Is64BitOperatingSystem -eq $false) { Fail "仅支持 Windows x64。" }

$scriptDir = Split-Path -Parent $PSCommandPath
$repoRoot = Split-Path -Parent $scriptDir
$originalPrivateKey = [Environment]::GetEnvironmentVariable('TAURI_SIGNING_PRIVATE_KEY')
$originalPublicKey = [Environment]::GetEnvironmentVariable('TAURI_UPDATER_PUBLIC_KEY')
$originalEmbeddedPublicKey = [Environment]::GetEnvironmentVariable('BB_ERP_UPDATE_PUBLIC_KEY')
Push-Location $repoRoot
$tempRoot = $null
$pendingDir = $null
try {
  # 使用当前进程变量，绝不把私钥、密码或规范化结果写入磁盘或报告。
  $privateKey = Get-EnvValueOrFile 'TAURI_SIGNING_PRIVATE_KEY'
  $privateKeyPassword = [string][Environment]::GetEnvironmentVariable('TAURI_SIGNING_PRIVATE_KEY_PASSWORD')
  if ([string]::IsNullOrWhiteSpace($privateKeyPassword)) { Fail '未配置环境变量 TAURI_SIGNING_PRIVATE_KEY_PASSWORD。' }
  $rawPublicKey = Get-EnvValueOrFile 'TAURI_UPDATER_PUBLIC_KEY'
  $normalizedPublicKey = ((& (Join-Path $scriptDir 'normalize-tauri-public-key.ps1') -Value $rawPublicKey) -join '').Trim()
  if ([string]::IsNullOrWhiteSpace($normalizedPublicKey)) { Fail '更新公钥规范化失败。' }
  $env:TAURI_SIGNING_PRIVATE_KEY = $privateKey.Trim()
  $env:TAURI_SIGNING_PRIVATE_KEY_PASSWORD = $privateKeyPassword
  $env:TAURI_UPDATER_PUBLIC_KEY = $normalizedPublicKey
  $env:BB_ERP_UPDATE_PUBLIC_KEY = $normalizedPublicKey

  foreach ($command in @('go','node','npm','cargo','rustc','gcc','cl')) { Require-Command $command }
  Require-File (Join-Path $repoRoot 'web/package-lock.json') 'web/package-lock.json 不存在。'
  Require-File (Join-Path $repoRoot 'client/package-lock.json') 'client/package-lock.json 不存在。'
  Require-File (Join-Path $repoRoot 'client/src-tauri/Cargo.toml') 'Tauri Cargo.toml 不存在。'
  Require-File (Join-Path $repoRoot 'cmd/server/main.go') '服务端入口不存在。'
  Require-File (Join-Path $repoRoot 'cmd/updater/main.go') '升级器入口不存在。'
  # 该查询不会启动浏览器；缺失时 Tauri 的实际构建也会失败，因此提前给出可读错误。
  if (-not (Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients\*' -ErrorAction SilentlyContinue | Where-Object { $_.name -eq '{F1E7D93E-2A3A-4D8B-9B0F-6EDC2E1A6D0B}' })) {
    Write-Warning '未能从注册表确认 WebView2 Runtime；若 Tauri 构建失败，请安装 WebView2 Evergreen Runtime。'
  }

  $resolvedOutput = if ([IO.Path]::IsPathRooted($OutputDir)) { $OutputDir } else { Join-Path $repoRoot $OutputDir }
  $finalDir = Join-Path $resolvedOutput ("v" + $Version)
  if (Test-Path -LiteralPath $finalDir) { Fail "输出目录已存在：$finalDir。请使用新版本或另一个 OutputDir，避免覆盖已有发布物。" }
  $tempRoot = Join-Path ([IO.Path]::GetTempPath()) ("bb-erp-package-" + [Guid]::NewGuid().ToString('N'))
  $buildRoot = Join-Path $tempRoot 'build'
  $stageRoot = Join-Path $tempRoot 'stage'
  New-Item -ItemType Directory -Force -Path $buildRoot, $stageRoot | Out-Null

  Write-Host "[1/7] 安装并构建 Web 与 Tauri 依赖..."
  Push-Location (Join-Path $repoRoot 'web'); try { npm ci; Assert-LastExit 'Web npm ci'; npm run build; Assert-LastExit 'Web npm run build' } finally { Pop-Location }
  Push-Location (Join-Path $repoRoot 'client'); try { npm ci; Assert-LastExit 'Client npm ci' } finally { Pop-Location }

  function Sign-And-Verify([string]$File) {
    $absolute = [IO.Path]::GetFullPath($File)
    Push-Location (Join-Path $repoRoot 'client')
    try { & npx --no-install tauri signer sign $absolute | Out-Null; Assert-LastExit "Tauri 签名 $([IO.Path]::GetFileName($File))" } finally { Pop-Location }
    $signaturePath = "$absolute.sig"; Require-File $signaturePath "Tauri 未生成签名：$signaturePath"
    $signature = (Get-Content -LiteralPath $signaturePath -Raw).Trim()
    Remove-Item -LiteralPath $signaturePath -Force
    if ($signature -notmatch '^[A-Za-z0-9+/=]+$') { Fail "签名格式无效：$([IO.Path]::GetFileName($File))" }
    & go run ./cmd/verify-update-signature -public-key $normalizedPublicKey -file $absolute -signature $signature
    Assert-LastExit "签名反向验证 $([IO.Path]::GetFileName($File))"
    return $signature
  }
  $keyChallenge = Join-Path $buildRoot 'signing-key-challenge.txt'
  Write-Utf8NoBom $keyChallenge ("bb-erp-signing-key-check:" + [Guid]::NewGuid().ToString('N'))
  [void](Sign-And-Verify $keyChallenge)

  Write-Host "[2/7] 构建 Windows 服务端和升级器..."
  $env:CGO_ENABLED = '1'; $env:CC = 'gcc'
  & go build -tags nodynamic -trimpath -ldflags "-s -w -X bb_erp_echo/internal/buildinfo.Version=$Version" -o (Join-Path $buildRoot 'bb-erp-server.exe') ./cmd/server
  Assert-LastExit 'Windows 服务端编译'
  $env:CGO_ENABLED = '0'; Remove-Item Env:CC -ErrorAction SilentlyContinue
  & go build -trimpath -ldflags '-s -w' -o (Join-Path $buildRoot 'bb-erp-updater.exe') ./cmd/updater
  Assert-LastExit 'Windows 升级器编译'
  & go build -trimpath -ldflags '-s -w' -o (Join-Path $buildRoot 'bb-erp-verify-update.exe') ./cmd/verify-update-signature
  Assert-LastExit 'Windows 更新签名验证器编译'

  Write-Host "[3/7] 构建带更新公钥的 Tauri NSIS 客户端..."
  $tauriConfigPath = Join-Path $buildRoot 'tauri-version.json'
  $tauriConfig = [ordered]@{ version = $Version; bundle = [ordered]@{ createUpdaterArtifacts = $true }; plugins = [ordered]@{ updater = [ordered]@{ pubkey = $normalizedPublicKey } } }
  Write-Utf8NoBom $tauriConfigPath ($tauriConfig | ConvertTo-Json -Depth 8 -Compress)
  $nsisOutputDir = Join-Path $repoRoot 'client/src-tauri/target/release/bundle/nsis'
  if (Test-Path -LiteralPath $nsisOutputDir) {
    Remove-Item -LiteralPath $nsisOutputDir -Recurse -Force
  }
  Push-Location (Join-Path $repoRoot 'client'); try { npm run desktop:build -- --bundles nsis --config $tauriConfigPath; Assert-LastExit 'Tauri NSIS 构建' } finally { Pop-Location }
  $clientExe = Join-Path $repoRoot 'client/src-tauri/target/release/bb_erp_client.exe'
  Require-File $clientExe "未找到 Tauri 客户端：$clientExe"
  $nsisCandidates = @(Get-ChildItem -LiteralPath $nsisOutputDir -Filter '*.exe' -Recurse)
  if ($nsisCandidates.Count -ne 1) { Fail "本次构建应生成且仅生成一个 NSIS 安装器，实际为 $($nsisCandidates.Count) 个。" }
  $nsis = $nsisCandidates[0]
  if ($null -eq $nsis) { Fail '未找到 Tauri NSIS 安装器。' }
  $portable = Join-Path $buildRoot 'bb-erp-client-windows-x86_64.exe'
  $nsisFile = Join-Path $buildRoot 'bb-erp-client-windows-x86_64-setup.exe'
  Copy-Item -LiteralPath $clientExe -Destination $portable -Force
  Copy-Item -LiteralPath $nsis.FullName -Destination $nsisFile -Force

  Write-Host "[4/7] 组装 all-in-one..."
  $serverStage = Join-Path $stageRoot 'server'
  $allStage = Join-Path $stageRoot 'all-in-one'
  New-Item -ItemType Directory -Force -Path (Join-Path $serverStage 'web'), (Join-Path $serverStage 'data'), (Join-Path $serverStage 'logs'), (Join-Path $serverStage 'updates/cache'), (Join-Path $serverStage 'updates/releases/active'), (Join-Path $serverStage 'static/uploads') | Out-Null
  Copy-Item (Join-Path $buildRoot 'bb-erp-server.exe'), (Join-Path $buildRoot 'bb-erp-updater.exe'), (Join-Path $buildRoot 'bb-erp-verify-update.exe') -Destination $serverStage -Force
  Copy-Item -LiteralPath (Join-Path $scriptDir 'activate-offline-update.ps1') -Destination (Join-Path $serverStage '激活离线更新.ps1') -Force
  Copy-TreeContents (Join-Path $repoRoot 'web/dist') (Join-Path $serverStage 'web/dist')
  Write-Utf8NoBom (Join-Path $serverStage 'update-public.key') $normalizedPublicKey
  Write-Utf8NoBom (Join-Path $serverStage 'updates/releases/active/README.txt') '将已验签的离线更新整包内容完整解压到此目录，再运行一键升级服务端.bat。'
  Write-Utf8NoBom (Join-Path $serverStage 'version.json') (([ordered]@{ version=$Version; server_version=$Version; client_version=$Version; update_source='directory'; release_dir='updates\releases\active'; manifest_url='' }) | ConvertTo-Json -Compress)
  $serverStart = @'
@echo off
setlocal
cd /d "%~dp0"
set BB_ERP_HTTP_HOST=0.0.0.0
set BB_ERP_HTTP_PORT=8080
set BB_ERP_APP_ENVIRONMENT=production
set BB_ERP_DATABASE_PATH=data\erp.db
set BB_ERP_LOG_DIR=logs
set BB_ERP_FILES_ROOT_DIR=static\uploads
set BB_ERP_WEB_ENABLED=true
set BB_ERP_WEB_DIST_DIR=web\dist
set BB_ERP_UPDATE_ENABLED=true
set BB_ERP_UPDATE_SOURCE=directory
set BB_ERP_UPDATE_RELEASE_DIR=updates\releases\active
set BB_ERP_UPDATE_CACHE_DIR=updates\cache
set BB_ERP_UPDATE_SIGNING_PUBLIC_KEY_FILE=update-public.key
bb-erp-server.exe
pause
'@
  Set-Content -LiteralPath (Join-Path $serverStage '启动服务端.bat') -Value $serverStart -Encoding ASCII
  $runner = '@echo off' + "`r`n" + 'cd /d "%~dp0"' + "`r`n" + 'bb-erp-updater.exe -release-dir "%~dp0updates\releases\active" -install-dir "%~dp0"' + "`r`n" + 'pause'
  Set-Content -LiteralPath (Join-Path $serverStage 'bb-erp-upgrade-runner.bat') -Value $runner -Encoding ASCII
  $loader = @'
@echo off
setlocal
cd /d "%~dp0"
if exist bb-erp-updater.pending.exe (
  move /y bb-erp-updater.pending.exe bb-erp-updater.exe >nul
  if errorlevel 1 exit /b 1
)
if exist bb-erp-upgrade-runner.pending.bat (
  move /y bb-erp-upgrade-runner.pending.bat bb-erp-upgrade-runner.bat >nul
  if errorlevel 1 exit /b 1
)
call bb-erp-upgrade-runner.bat
exit /b %ERRORLEVEL%
'@
  Set-Content -LiteralPath (Join-Path $serverStage '一键升级服务端.bat') -Value $loader -Encoding ASCII
  Copy-TreeContents $serverStage (Join-Path $allStage 'server')
  New-Item -ItemType Directory -Force -Path (Join-Path $allStage 'client'), (Join-Path $allStage 'installer') | Out-Null
  Copy-Item -LiteralPath $clientExe -Destination (Join-Path $allStage 'client/bb_erp_client.exe') -Force
  Write-Utf8NoBom (Join-Path $allStage 'client/bb-erp-portable.json') (([ordered]@{ layout_version=1; version=$Version; install_mode='portable' }) | ConvertTo-Json -Compress)
  Copy-Item -LiteralPath $nsisFile -Destination (Join-Path $allStage 'installer/博邦 ERP NSIS 安装器.exe') -Force
  Set-Content -LiteralPath (Join-Path $allStage '启动系统.bat') -Value ('@echo off' + "`r`n" + 'start "BB ERP Server" /d "%~dp0server" "%~dp0server\启动服务端.bat"' + "`r`n" + 'timeout /t 3 /nobreak >nul' + "`r`n" + 'start "BB ERP Client" "%~dp0client\bb_erp_client.exe"') -Encoding ASCII
  Set-Content -LiteralPath (Join-Path $allStage 'README.txt') -Value "博邦 ERP Windows 全量安装包`r`n版本：$Version`r`n解压后运行 启动系统.bat。数据保存在 server\data。" -Encoding UTF8
  $allZip = Join-Path $buildRoot ("bb-erp-all-in-one-windows-v$Version.zip")
  Compress-Directory $allStage $allZip

  $outputs = New-Object System.Collections.Generic.List[object]
  $outputs.Add($allZip)
  if ($Target -eq 'All') {
    Write-Host "[5/7] 生成并验证离线更新整包签名..."
    $serverZip = Join-Path $buildRoot 'bb-erp-server-windows.zip'
    Compress-Directory $serverStage $serverZip
    $serverSignature = Sign-And-Verify $serverZip
    $portableSignature = Sign-And-Verify $portable
    $nsisSignature = Sign-And-Verify $nsisFile
    $payload = [ordered]@{ protocol_version=2; version=$Version; target='windows-x86_64'; layout_version=1; full=[ordered]@{
      nsis=[ordered]@{ kind='nsis'; url=[IO.Path]::GetFileName($nsisFile); size=(Get-Item -LiteralPath $nsisFile).Length; sha256=(Get-FileSha256 $nsisFile); signature=$nsisSignature }
      portable=[ordered]@{ kind='portable'; url=[IO.Path]::GetFileName($portable); size=(Get-Item -LiteralPath $portable).Length; sha256=(Get-FileSha256 $portable); signature=$portableSignature }
    } }
    $payloadPath = Join-Path $buildRoot 'client-update-v2-payload.json'
    Write-Utf8NoBom $payloadPath ($payload | ConvertTo-Json -Depth 8 -Compress)
    $payloadSignature = Sign-And-Verify $payloadPath
    $payloadBase64 = [Convert]::ToBase64String([IO.File]::ReadAllBytes($payloadPath))
    $manifest = [ordered]@{ version=$Version; published_at=(Get-Date).ToUniversalTime().ToString('o'); notes='BB ERP offline Windows release'; server=[ordered]@{ version=$Version; url=[IO.Path]::GetFileName($serverZip); sha256=(Get-FileSha256 $serverZip); size=(Get-Item -LiteralPath $serverZip).Length; signature=$serverSignature }; client_update_v2=[ordered]@{ payload=$payloadBase64; signature=$payloadSignature } }
    $offlineStage = Join-Path $stageRoot 'offline-update'
    New-Item -ItemType Directory -Force -Path $offlineStage | Out-Null
    Write-Utf8NoBom (Join-Path $offlineStage 'update-manifest.json') ($manifest | ConvertTo-Json -Depth 8)
    Write-Utf8NoBom (Join-Path $offlineStage 'update-public.key') $normalizedPublicKey
    Copy-Item -LiteralPath $serverZip, $portable, $nsisFile -Destination $offlineStage -Force
    $checksumLines = Get-ChildItem -LiteralPath $offlineStage -File | Sort-Object Name | ForEach-Object { "$(Get-FileSha256 $_.FullName)  $($_.Name)" }
    Write-Utf8NoBom (Join-Path $offlineStage 'checksums.sha256') ($checksumLines -join "`n")
    Set-Content -LiteralPath (Join-Path $offlineStage 'README.txt') -Value "将本目录完整解压到服务器 updates\releases\incoming\v$Version。然后在已安装的 server 目录运行：.\激活离线更新.ps1 -IncomingDir updates\releases\incoming\v$Version。不要运行 incoming 中的任何程序，也不要直接覆盖 active、data、logs、cache 或 uploads。" -Encoding UTF8
    $offlineZip = Join-Path $buildRoot ("bb-erp-offline-update-v$Version.zip")
    Compress-Directory $offlineStage $offlineZip
    $outputs.Add($offlineZip)
  } else { Write-Host '[5/7] 已跳过离线更新整包（Target=AllInOne）。' }

  Write-Host '[6/7] 解压校验最终 ZIP...'
  $verify = Join-Path $tempRoot 'verify'; New-Item -ItemType Directory -Force -Path $verify | Out-Null
  foreach ($archive in $outputs) {
    $destination = Join-Path $verify ([IO.Path]::GetFileNameWithoutExtension($archive))
    Expand-Archive -LiteralPath $archive -DestinationPath $destination -Force
  }
  Require-File (Join-Path $verify ("bb-erp-all-in-one-windows-v$Version/server/bb-erp-server.exe")) 'all-in-one 缺少服务端。'
  Require-File (Join-Path $verify ("bb-erp-all-in-one-windows-v$Version/client/bb_erp_client.exe")) 'all-in-one 缺少客户端。'
  Require-File (Join-Path $verify ("bb-erp-all-in-one-windows-v$Version/installer/博邦 ERP NSIS 安装器.exe")) 'all-in-one 缺少安装器。'
  Require-File (Join-Path $verify ("bb-erp-all-in-one-windows-v$Version/server/update-public.key")) 'all-in-one 缺少更新公钥。'
  if ($Target -eq 'All') {
    $verifiedOffline = Join-Path $verify ("bb-erp-offline-update-v$Version")
    $verifiedManifestPath = Join-Path $verifiedOffline 'update-manifest.json'
    Require-File $verifiedManifestPath '离线更新包缺少 manifest。'
    foreach ($line in Get-Content -LiteralPath (Join-Path $verifiedOffline 'checksums.sha256')) {
      if ($line -notmatch '^([0-9a-fA-F]{64})  ([^\\/]+)$') { Fail "离线更新校验和格式无效：$line" }
      $verifiedFile = Join-Path $verifiedOffline $Matches[2]
      Require-File $verifiedFile "离线更新缺少校验文件：$($Matches[2])"
      if (-not (Get-FileSha256 $verifiedFile).Equals($Matches[1], [StringComparison]::OrdinalIgnoreCase)) { Fail "离线更新重新解压后 SHA-256 不匹配：$($Matches[2])" }
    }
    $verifiedManifest = Get-Content -LiteralPath $verifiedManifestPath -Raw | ConvertFrom-Json
    & go run ./cmd/verify-update-signature -public-key $normalizedPublicKey -file (Join-Path $verifiedOffline $verifiedManifest.server.url) -signature $verifiedManifest.server.signature
    Assert-LastExit '重新验证离线服务端签名'
    $verifiedPayloadBytes = [Convert]::FromBase64String([string]$verifiedManifest.client_update_v2.payload)
    $verifiedPayloadPath = Join-Path $verify 'client-update-v2-payload.json'
    [IO.File]::WriteAllBytes($verifiedPayloadPath, $verifiedPayloadBytes)
    & go run ./cmd/verify-update-signature -public-key $normalizedPublicKey -file $verifiedPayloadPath -signature $verifiedManifest.client_update_v2.signature
    Assert-LastExit '重新验证离线客户端 payload 签名'
    $verifiedPayload = [Text.Encoding]::UTF8.GetString($verifiedPayloadBytes) | ConvertFrom-Json
    foreach ($artifact in @($verifiedPayload.full.nsis, $verifiedPayload.full.portable)) {
      $artifactPath = Join-Path $verifiedOffline $artifact.url
      Require-File $artifactPath "离线更新缺少客户端资源：$($artifact.url)"
      if ((Get-Item -LiteralPath $artifactPath).Length -ne [int64]$artifact.size) { Fail "离线客户端资源大小不匹配：$($artifact.url)" }
      if (-not (Get-FileSha256 $artifactPath).Equals([string]$artifact.sha256, [StringComparison]::OrdinalIgnoreCase)) { Fail "离线客户端资源 SHA-256 不匹配：$($artifact.url)" }
      & go run ./cmd/verify-update-signature -public-key $normalizedPublicKey -file $artifactPath -signature $artifact.signature
      Assert-LastExit "重新验证离线客户端签名 $($artifact.url)"
    }
  }

  Write-Host '[7/7] 写入校验和和构建报告...'
  New-Item -ItemType Directory -Force -Path $resolvedOutput | Out-Null
  $pendingDir = Join-Path $resolvedOutput (".v$Version.pending-" + [Guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $pendingDir | Out-Null
  foreach ($file in $outputs) {
    $destination = Join-Path $pendingDir ([IO.Path]::GetFileName($file))
    Copy-Item -LiteralPath $file -Destination $destination -Force
    Write-Utf8NoBom ($destination + '.sha256') ((Get-FileSha256 $destination) + '  ' + [IO.Path]::GetFileName($destination))
  }
  $report = [ordered]@{ version=$Version; target=$Target; built_at=(Get-Date).ToUniversalTime().ToString('o'); files=@(Get-ChildItem -LiteralPath $pendingDir -File | Sort-Object Name | ForEach-Object { [ordered]@{ name=$_.Name; size=$_.Length; sha256=(Get-FileSha256 $_.FullName) } }) }
  Write-Utf8NoBom (Join-Path $pendingDir 'build-report.json') ($report | ConvertTo-Json -Depth 6)
  Move-Item -LiteralPath $pendingDir -Destination $finalDir
  Write-Host "完成：$finalDir"
} finally {
  Pop-Location
  if ($tempRoot -and (Test-Path -LiteralPath $tempRoot)) { Remove-Item -LiteralPath $tempRoot -Recurse -Force -ErrorAction SilentlyContinue }
  if ($pendingDir -and (Test-Path -LiteralPath $pendingDir)) { Remove-Item -LiteralPath $pendingDir -Recurse -Force -ErrorAction SilentlyContinue }
  Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Remove-Item Env:CC -ErrorAction SilentlyContinue
  if ($null -eq $originalPrivateKey) { Remove-Item Env:TAURI_SIGNING_PRIVATE_KEY -ErrorAction SilentlyContinue } else { $env:TAURI_SIGNING_PRIVATE_KEY = $originalPrivateKey }
  if ($null -eq $originalPublicKey) { Remove-Item Env:TAURI_UPDATER_PUBLIC_KEY -ErrorAction SilentlyContinue } else { $env:TAURI_UPDATER_PUBLIC_KEY = $originalPublicKey }
  if ($null -eq $originalEmbeddedPublicKey) { Remove-Item Env:BB_ERP_UPDATE_PUBLIC_KEY -ErrorAction SilentlyContinue } else { $env:BB_ERP_UPDATE_PUBLIC_KEY = $originalEmbeddedPublicKey }
}
