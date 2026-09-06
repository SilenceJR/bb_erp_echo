#!/usr/bin/env bash
# Build the signed, manually deployed Windows single-EXE client update bundle.
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
client_dir="${TAURI_CLIENT_DIR:-$repo_root/client}"
source_exe="${RELEASE_CLIENT_EXE:?RELEASE_CLIENT_EXE is required}"
output_dir="${RELEASE_OUTPUT_DIR:?RELEASE_OUTPUT_DIR is required}"
version="${RELEASE_VERSION:?RELEASE_VERSION is required}"
public_key="${TAURI_UPDATER_PUBLIC_KEY:?TAURI_UPDATER_PUBLIC_KEY is required}"
: "${TAURI_SIGNING_PRIVATE_KEY:?TAURI_SIGNING_PRIVATE_KEY is required}"
: "${TAURI_SIGNING_PRIVATE_KEY_PASSWORD:?TAURI_SIGNING_PRIVATE_KEY_PASSWORD is required}"

test -s "$source_exe"
command -v jq >/dev/null
command -v sha256sum >/dev/null
mkdir -p "$output_dir"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

sign_file() {
  local file="$1"
  local absolute_file
  absolute_file="$(cd "$(dirname "$file")" && pwd)/$(basename "$file")"
  (cd "$client_dir" && npx --no-install tauri signer sign "$absolute_file" >/dev/null)
  local signature
  signature="$(tr -d '\r\n' <"$absolute_file.sig")"
  rm -f "$absolute_file.sig"
  (cd "$repo_root" && go run ./cmd/verify-update-signature \
    -public-key "$public_key" -file "$absolute_file" -signature "$signature")
  printf '%s' "$signature"
}

target_exe="$output_dir/bb_erp_client.exe"
cp "$source_exe" "$target_exe"
size="$(wc -c <"$target_exe" | tr -d '[:space:]')"
sha256="$(sha256sum "$target_exe" | awk '{print $1}')"
file_signature="$(sign_file "$target_exe")"

payload="$work_dir/client-update-payload.json"
jq -cn \
  --arg version "$version" \
  --arg target "windows-x86_64" \
  --arg sha256 "$sha256" \
  --arg signature "$file_signature" \
  --argjson size "$size" \
  '{version:$version,target:$target,artifact:{kind:"portable",size:$size,sha256:$sha256,signature:$signature}}' \
  >"$payload"

payload_b64="$(base64 <"$payload" | tr -d '\r\n')"
payload_signature="$(sign_file "$payload")"
jq -cn --arg payload "$payload_b64" --arg signature "$payload_signature" \
  '{payload:$payload,signature:$signature}' >"$output_dir/client-update.json"

(cd "$repo_root" && go run ./cmd/verify-update-signature \
  -public-key "$public_key" -file "$target_exe" -signature "$file_signature")

printf '%s\n' \
  '博邦 ERP 客户端更新投放包' \
  '1. 停止覆盖 client-update.json。' \
  '2. 先复制 bb_erp_client.exe 到服务器同级 client 目录。' \
  '3. 最后复制 client-update.json；服务端验证成功后才会发布。' \
  >"$output_dir/README.txt"

echo "Signed single-EXE client update bundle prepared for $version."
