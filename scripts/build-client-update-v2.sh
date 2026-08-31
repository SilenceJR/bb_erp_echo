#!/usr/bin/env bash
# Builds the signed Windows full-only client_update_v2 manifest section.
# The stable manifest is read only for downgrade protection; clients never read
# release-host URLs and download verified artifacts through their LAN ERP server.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
# shellcheck source=release-semver.sh
source "$script_dir/release-semver.sh"

asset_dir="${RELEASE_ASSET_DIR:?RELEASE_ASSET_DIR is required}"
manifest="${RELEASE_MANIFEST_FILE:-$asset_dir/update-manifest.json}"
version="${RELEASE_VERSION:?RELEASE_VERSION is required}"
tag="${RELEASE_TAG:?RELEASE_TAG is required}"
base_url="${RELEASE_BASE_URL:?RELEASE_BASE_URL is required}"
stable_manifest_url="${RELEASE_STABLE_MANIFEST_URL:?RELEASE_STABLE_MANIFEST_URL is required}"
client_dir="${TAURI_CLIENT_DIR:-client}"
layout_version="${RELEASE_LAYOUT_VERSION:-1}"
portable_file="${RELEASE_PORTABLE_FILE:?RELEASE_PORTABLE_FILE is required}"
nsis_file="${RELEASE_NSIS_FILE:?RELEASE_NSIS_FILE is required}"
server_file="${RELEASE_SERVER_FILE:?RELEASE_SERVER_FILE is required}"
public_key="${TAURI_UPDATER_PUBLIC_KEY:?TAURI_UPDATER_PUBLIC_KEY is required}"

test -s "$manifest"
test -s "$portable_file"
test -s "$nsis_file"
test -s "$server_file"
command -v jq >/dev/null
command -v curl >/dev/null
command -v sha256sum >/dev/null

if [[ "$tag" == *-* || "$version" == *-* ]]; then
  echo "Only vMAJOR.MINOR.PATCH releases are accepted by the current update channel." >&2
  exit 1
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

file_size() { wc -c <"$1" | tr -d '[:space:]'; }
file_hash() { sha256sum "$1" | awk '{print $1}'; }
file_url() { printf '%s/%s' "${base_url%/}" "$(basename "$1")"; }
verify_file() {
  local file="$1"
  local signature="$2"
  (cd "$repo_root" && go run ./cmd/verify-update-signature \
    -public-key "$public_key" -file "$file" -signature "$signature")
}
sign_file() {
  local file="$1"
  local absolute_file
  absolute_file="$(cd "$(dirname "$file")" && pwd)/$(basename "$file")"
  (cd "$client_dir" && npx --no-install tauri signer sign "$absolute_file" >/dev/null)
  local signature
  signature="$(tr -d '\r\n' <"$absolute_file.sig")"
  rm -f "$absolute_file.sig"
  [[ "$signature" =~ ^[A-Za-z0-9+/=]+$ ]] || {
    echo "Tauri signer returned an invalid signature for $file" >&2
    return 1
  }
  printf '%s' "$signature" | base64 --decode >/dev/null 2>&1 || {
    echo "Tauri signer returned a non-decodable signature for $file" >&2
    return 1
  }
  verify_file "$absolute_file" "$signature" || {
    echo "Tauri signing private key does not match TAURI_UPDATER_PUBLIC_KEY for $file" >&2
    return 1
  }
  printf '%s' "$signature"
}

stable_manifest="$work_dir/stable-manifest.json"
stable_http_code="$(curl --silent --show-error --location --retry 3 --retry-all-errors \
  --output "$stable_manifest" --write-out '%{http_code}' "$stable_manifest_url" || true)"
if [[ "$stable_http_code" == "200" ]]; then
  stable_version="$(jq -er '.version' "$stable_manifest")" || {
    echo "Stable manifest has no valid version; refusing to publish." >&2
    exit 1
  }
  comparison="$(semver_compare "$version" "$stable_version")" || {
    echo "Invalid SemVer: candidate=$version stable=$stable_version" >&2
    exit 1
  }
  if [[ "$comparison" != "1" ]]; then
    echo "Release version must be greater than stable version: candidate=$version stable=$stable_version" >&2
    exit 1
  fi
elif [[ "$stable_http_code" != "404" ]]; then
  echo "Unable to read stable manifest (HTTP ${stable_http_code:-000}); refusing to publish." >&2
  exit 1
fi

portable_hash="$(file_hash "$portable_file")"
portable_size="$(file_size "$portable_file")"
nsis_hash="$(file_hash "$nsis_file")"
nsis_size="$(file_size "$nsis_file")"
server_signature="$(sign_file "$server_file")"
portable_signature="$(sign_file "$portable_file")"
nsis_signature="$(sign_file "$nsis_file")"

payload_file="$work_dir/client-update-v2-payload.json"
jq -cn \
  --arg version "$version" --arg target "windows-x86_64" --argjson layout "$layout_version" \
  --arg nsis_url "$(file_url "$nsis_file")" --arg nsis_sha "$nsis_hash" --arg nsis_signature "$nsis_signature" --argjson nsis_size "$nsis_size" \
  --arg portable_url "$(file_url "$portable_file")" --arg portable_sha "$portable_hash" --arg portable_signature "$portable_signature" --argjson portable_size "$portable_size" \
  '{protocol_version:2,version:$version,target:$target,layout_version:$layout,full:{nsis:{kind:"nsis",url:$nsis_url,size:$nsis_size,sha256:$nsis_sha,signature:$nsis_signature},portable:{kind:"portable",url:$portable_url,size:$portable_size,sha256:$portable_sha,signature:$portable_signature}}}' \
  >"$payload_file"

payload_b64="$(base64 <"$payload_file" | tr -d '\r\n')"
payload_signature="$(sign_file "$payload_file")"
tmp_manifest="$work_dir/update-manifest.json"
jq --arg server_signature "$server_signature" --arg payload "$payload_b64" --arg signature "$payload_signature" \
  'del(.client) | .server.signature = $server_signature | .client_update_v2 = {payload:$payload, signature:$signature}' \
  "$manifest" >"$tmp_manifest"
mv "$tmp_manifest" "$manifest"

echo "Signed Windows full-only client update manifest prepared for $tag."
