param(
  [Parameter(Mandatory = $true)]
  [AllowEmptyString()]
  [string] $Value
)

$ErrorActionPreference = "Stop"

function Fail([string] $Message) {
  throw "TAURI_UPDATER_PUBLIC_KEY 无效：$Message"
}

$keyLine = $null
$commentKeyId = $null
$inputText = $Value.Trim()
if (-not $inputText) {
  Fail "内容不能为空"
}

$publicText = $null
if ($inputText -match 'untrusted comment: minisign public key') {
  $publicText = $inputText
} else {
  $candidate = $inputText -replace '\s', ''
  if ($candidate -notmatch '^[A-Za-z0-9+/]+={0,2}$') {
    Fail "格式必须是 Base64 内容"
  }
  try {
    $decoded = [Convert]::FromBase64String($candidate)
  } catch {
    Fail "公钥 Base64 无法解码"
  }
  if ($decoded.Length -eq 42) {
    $keyLine = $candidate
  } else {
    try {
      $publicText = [Text.Encoding]::UTF8.GetString($decoded)
    } catch {
      Fail "Base64 封装无法转换为公钥文本"
    }
  }
}

if ($publicText) {
  $commentMatch = [regex]::Match($publicText, 'untrusted comment: minisign public key ([0-9A-Fa-f]{16})')
  $keyMatches = [regex]::Matches($publicText, '[A-Za-z0-9+/]{50,}={0,2}')
  if (-not $commentMatch.Success -or $keyMatches.Count -ne 1) {
    Fail "公钥文本必须包含匹配的 Minisign 注释和 Base64 公钥行"
  }
  $commentKeyId = $commentMatch.Groups[1].Value
  $keyLine = $keyMatches[0].Value
}

if ($keyLine -notmatch '^[A-Za-z0-9+/]+={0,2}$') {
  Fail "公钥内容不是合法 Base64"
}

try {
  $keyBytes = [Convert]::FromBase64String($keyLine)
} catch {
  Fail "公钥 Base64 无法解码"
}

if ($keyBytes.Length -ne 42) {
  Fail "解码后长度应为 42 字节，实际为 $($keyBytes.Length) 字节"
}

$keyIdBytes = $keyBytes[2..9]
$keyId = (($keyIdBytes | ForEach-Object { $_.ToString("x2") }) -join "")
if ($commentKeyId -and $commentKeyId.ToLowerInvariant() -ne $keyId) {
  Fail "注释中的 key identifier 与公钥内容不一致"
}

$canonicalText = "untrusted comment: minisign public key $keyId`n$keyLine`n"
[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($canonicalText))
