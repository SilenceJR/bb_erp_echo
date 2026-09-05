#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
script="$script_dir/windows-package.ps1"

test -s "$script"

require() {
  local pattern="$1"
  rg -q "$pattern" "$script" || {
    echo "windows-package.ps1 missing contract: $pattern" >&2
    exit 1
  }
}

require 'ValidateSet\("AllInOne", "All"\)'
require '\[string\]\$Target = "AllInOne"'
require '\$Target -eq '\''All'\'''
require 'TAURI_SIGNING_PRIVATE_KEY'
require 'TAURI_SIGNING_PRIVATE_KEY_PASSWORD'
require 'TAURI_UPDATER_PUBLIC_KEY'
require 'normalize-tauri-public-key\.ps1'
require 'BB_ERP_UPDATE_PUBLIC_KEY'
require 'createUpdaterArtifacts = \$true'
require 'BB_ERP_UPDATE_SOURCE=directory'
require 'updates\\releases\\active'
require 'client_update_v2'
require 'bb-erp-all-in-one-windows-v\$Version\.zip'
require 'bb-erp-offline-update-v\$Version\.zip'
require 'Move-Item -LiteralPath \$pendingDir -Destination \$finalDir'
require 'activate-offline-update\.ps1'

trusted_activation="$script_dir/activate-offline-update.ps1"
test -s "$trusted_activation"
rg -q '\$trustedRoot = \$PSScriptRoot' "$trusted_activation"
rg -q 'bb-erp-verify-update\.exe' "$trusted_activation"
rg -q '公钥与已安装信任根不一致' "$trusted_activation"
rg -q 'foreach \(\$name in \$releaseFiles\)' "$trusted_activation"
rg -q 'Test-Release \$pending' "$trusted_activation"
if rg -q 'Get-ChildItem.*\| Copy-Item' "$trusted_activation"; then
  echo 'activation must copy only the fixed release whitelist' >&2
  exit 1
fi
if rg -q 'Join-Path \$sourceDir .bb-erp-verify-update\.exe.|Join-Path \$sourceDir .update-public\.key..*-public-key' "$trusted_activation"; then
  echo 'activation must not trust or execute verification components from incoming' >&2
  exit 1
fi

if rg -q 'GITEE_TOKEN|git push|gh release|Invoke-WebRequest|Start-BitsTransfer' "$script"; then
  echo 'windows-package.ps1 must not publish or download release artifacts' >&2
  exit 1
fi

echo 'windows-package static contract checks passed'
