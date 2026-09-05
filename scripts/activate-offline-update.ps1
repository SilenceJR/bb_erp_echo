[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)]
  [string]$IncomingDir,

  [string]$ActiveDir = ''
)

# 本脚本、验证器和可信公钥必须位于已安装且受 ACL 保护的 server 目录。
# incoming 目录只被当作不可信数据，绝不执行其中的 EXE、BAT 或 PowerShell。
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$trustedRoot = $PSScriptRoot
$sourceDir = [IO.Path]::GetFullPath($IncomingDir)
if ([string]::IsNullOrWhiteSpace($ActiveDir)) {
  $ActiveDir = Join-Path $trustedRoot 'updates\releases\active'
}
$ActiveDir = [IO.Path]::GetFullPath($ActiveDir)
$verify = Join-Path $trustedRoot 'bb-erp-verify-update.exe'
$trustedKeyFile = Join-Path $trustedRoot 'update-public.key'
foreach ($trusted in @($verify, $trustedKeyFile)) {
  if (-not (Test-Path -LiteralPath $trusted -PathType Leaf)) { throw "已安装的可信验证组件不存在：$trusted" }
}
$trustedKey = (Get-Content -LiteralPath $trustedKeyFile -Raw).Trim()
$releaseFiles = @('update-manifest.json','update-public.key','checksums.sha256','bb-erp-server-windows.zip','bb-erp-client-windows-x86_64.exe','bb-erp-client-windows-x86_64-setup.exe')
$hashedFiles = @('update-manifest.json','update-public.key','bb-erp-server-windows.zip','bb-erp-client-windows-x86_64.exe','bb-erp-client-windows-x86_64-setup.exe')
function Resolve-ReleaseResource([string]$Root, [string]$Name) {
  if ([string]::IsNullOrWhiteSpace($Name) -or $Name.Contains('..') -or [IO.Path]::GetFileName($Name) -ne $Name) {
    throw "资源必须是 incoming 当前目录内的普通文件名：$Name"
  }
  $resolved = Join-Path $Root $Name
  if (-not (Test-Path -LiteralPath $resolved -PathType Leaf)) { throw "资源不存在：$Name" }
  if (((Get-Item -LiteralPath $resolved -Force).Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0) { throw "资源不能是符号链接或 reparse point：$Name" }
  return $resolved
}
function Verify-SignedFile([string]$File, [string]$Signature) {
  & $verify -public-key $trustedKey -file $File -signature $Signature | Out-Null
  if ($LASTEXITCODE -ne 0) { throw "Minisign 验签失败：$File" }
}
function Test-Release([string]$Root) {
  $rootItem = Get-Item -LiteralPath $Root -Force
  if (-not $rootItem.PSIsContainer -or (($rootItem.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0)) { throw '发布根目录不能是符号链接或 reparse point。' }
  foreach ($required in $releaseFiles) { [void](Resolve-ReleaseResource $Root $required) }
  $incomingKey = (Get-Content -LiteralPath (Join-Path $Root 'update-public.key') -Raw).Trim()
  if (-not $trustedKey.Equals($incomingKey, [StringComparison]::Ordinal)) { throw '离线包公钥与已安装信任根不一致；当前版本不支持密钥轮换。' }
  $seen = @{}
  foreach ($line in Get-Content -LiteralPath (Join-Path $Root 'checksums.sha256')) {
    if ($line -notmatch '^([0-9a-fA-F]{64})  ([^\\/]+)$') { throw "校验和清单格式无效：$line" }
    $name = $Matches[2]
    if ($name -notin $hashedFiles -or $seen.ContainsKey($name)) { throw "校验和清单包含意外或重复文件：$name" }
    $seen[$name] = $true
    $actual = (Get-FileHash -LiteralPath (Resolve-ReleaseResource $Root $name) -Algorithm SHA256).Hash
    if (-not $actual.Equals($Matches[1], [StringComparison]::OrdinalIgnoreCase)) { throw "SHA-256 不匹配：$name" }
  }
  foreach ($name in $hashedFiles) { if (-not $seen.ContainsKey($name)) { throw "校验和清单缺少文件：$name" } }
  $manifest = Get-Content -LiteralPath (Join-Path $Root 'update-manifest.json') -Raw | ConvertFrom-Json
  Verify-SignedFile (Resolve-ReleaseResource $Root ([string]$manifest.server.url)) $manifest.server.signature
  $payloadBytes = [Convert]::FromBase64String([string]$manifest.client_update_v2.payload)
  $payloadPath = Join-Path $env:TEMP ("bb-erp-payload-" + [Guid]::NewGuid().ToString('N') + '.json')
  try {
    [IO.File]::WriteAllBytes($payloadPath, $payloadBytes)
    Verify-SignedFile $payloadPath $manifest.client_update_v2.signature
    $payload = [Text.Encoding]::UTF8.GetString($payloadBytes) | ConvertFrom-Json
    Verify-SignedFile (Resolve-ReleaseResource $Root ([string]$payload.full.nsis.url)) $payload.full.nsis.signature
    Verify-SignedFile (Resolve-ReleaseResource $Root ([string]$payload.full.portable.url)) $payload.full.portable.signature
  } finally { Remove-Item -LiteralPath $payloadPath -Force -ErrorAction SilentlyContinue }
  return $manifest
}
$manifest = Test-Release $sourceDir
$activeParent = Split-Path -Parent $ActiveDir
New-Item -ItemType Directory -Force -Path $activeParent | Out-Null
$pending = Join-Path $activeParent ('.active.pending-' + [Guid]::NewGuid().ToString('N'))
$backup = Join-Path $activeParent ('previous-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
try {
  New-Item -ItemType Directory -Path $pending | Out-Null
  foreach ($name in $releaseFiles) { Copy-Item -LiteralPath (Resolve-ReleaseResource $sourceDir $name) -Destination $pending -Force }
  $manifest = Test-Release $pending
  [IO.File]::WriteAllText((Join-Path $pending '.release-ready'), [string]$manifest.version, (New-Object Text.UTF8Encoding($false)))
  if (Test-Path -LiteralPath $ActiveDir) { Move-Item -LiteralPath $ActiveDir -Destination $backup }
  try { Move-Item -LiteralPath $pending -Destination $ActiveDir } catch {
    if (Test-Path -LiteralPath $backup) { Move-Item -LiteralPath $backup -Destination $ActiveDir }
    throw
  }
  Write-Host "离线更新已激活：$ActiveDir"
  if (Test-Path -LiteralPath $backup) { Write-Host "上一版本保留在：$backup" }
} finally {
  if (Test-Path -LiteralPath $pending) { Remove-Item -LiteralPath $pending -Recurse -Force -ErrorAction SilentlyContinue }
}
