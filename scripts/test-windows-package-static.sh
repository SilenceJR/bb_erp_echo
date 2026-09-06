#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
package="$script_dir/windows-package.ps1"
manifest="$script_dir/build-client-update.sh"
for pattern in 'bb-erp-client-update-windows-v\$Version.zip' 'bb-erp-all-in-one-windows-v\$Version.zip' 'client-update.json' 'bb_erp_client.exe' 'update-public.key'; do
  rg -q "$pattern" "$package" "$manifest" || { echo "windows package missing: $pattern" >&2; exit 1; }
done
for pattern in 'version:\$version' 'windows-x86_64' 'kind:"portable"' 'TAURI_SIGNING_PRIVATE_KEY' 'verify-update-signature'; do
  rg -q "$pattern" "$manifest" || { echo "client update bundle missing: $pattern" >&2; exit 1; }
done
if rg -qi 'nsis|msi|gitee|manifest_url|cmd/updater|activate-offline' "$package" "$manifest"; then
  echo 'offline packaging still contains a removed updater or publishing path' >&2
  exit 1
fi
echo 'Windows single-EXE packaging contract checks passed'
