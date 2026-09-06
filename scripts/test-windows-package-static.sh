#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
package="$script_dir/windows-package.ps1"
manifest="$script_dir/build-client-update.sh"
for pattern in 'bb-erp-client-update-windows-v\$Version.zip' 'bb-erp-all-in-one-windows-v\$Version.zip' 'client-update.json' 'bb_erp_client.exe' 'update-public.key'; do
  rg -q "$pattern" "$package" "$manifest" || { echo "windows package missing: $pattern" >&2; exit 1; }
done
for pattern in './cmd/server-tray' 'H=windowsgui' 'BB_ERP_LOG_CONSOLE.*false' 'WinHttp\.WinHttpRequest\.5\.1' '启动服务端\.vbs' '启动系统\.vbs' 'windres.*server-tray\.rc'; do
  rg -q "$pattern" "$package" || { echo "Windows tray package missing: $pattern" >&2; exit 1; }
done
if rg -q '^pause$|timeout /t 3|启动服务端\.bat|启动系统\.bat' "$package"; then
  echo 'Windows tray package still contains the blocking console or fixed readiness delay' >&2
  exit 1
fi
for pattern in 'version:\$version' 'windows-x86_64' 'kind:"portable"' 'TAURI_SIGNING_PRIVATE_KEY' 'verify-update-signature'; do
  rg -q "$pattern" "$manifest" || { echo "client update bundle missing: $pattern" >&2; exit 1; }
done
if rg -qi 'nsis|msi|gitee|manifest_url|cmd/updater|activate-offline' "$package" "$manifest"; then
  echo 'offline packaging still contains a removed updater or publishing path' >&2
  exit 1
fi
echo 'Windows single-EXE packaging contract checks passed'
