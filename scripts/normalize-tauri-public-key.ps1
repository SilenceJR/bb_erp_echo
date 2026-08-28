param(
  [Parameter(Mandatory = $true)]
  [AllowEmptyString()]
  [string] $Value
)

$ErrorActionPreference = "Stop"

function Fail([string] $Message) {
  throw "TAURI_UPDATER_PUBLIC_KEY 无效：$Message"
}

$lines = @($Value -split "`r?`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ })
if ($lines.Count -eq 0 -or $lines.Count -gt 2) {
  Fail "必须是公钥 Base64 行、两行 Minisign 公钥文本，或其 Base64 封装"
}

$keyLine = $null
$commentKeyId = $null
$envelopeDecoded = $null

if ($lines.Count -eq 2 -and $lines[0] -match '^untrusted comment: minisign public key ([0-9A-Fa-f]{16})$') {
  $commentKeyId = $Matches[1]
  $keyLine = $lines[1]
} elseif ($lines.Count -eq 1 -and $lines[0] -match '^[A-Za-z0-9+/]+={0,2}$') {
  $candidate = $lines[0]
  try {
    $decoded = [Convert]::FromBase64String($candidate)
    $decodedText = [Text.Encoding]::UTF8.GetString($decoded)
    $decodedLines = @($decodedText -split "`r?`n" | ForEach-Object { $_.Trim() } | Where-Object { $_ })
    if ($decodedLines.Count -eq 2 -and $decodedLines[0] -match '^untrusted comment: minisign public key ([0-9A-Fa-f]{16})$') {
      $commentKeyId = $Matches[1]
      $keyLine = $decodedLines[1]
      $envelopeDecoded = $decodedText
    } else {
      $keyLine = $candidate
    }
  } catch {
    $keyLine = $candidate
  }
} else {
  Fail "格式必须是 Base64 内容"
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
