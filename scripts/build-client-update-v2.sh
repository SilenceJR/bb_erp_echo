#!/usr/bin/env bash
# Builds the signed client_update_v2 manifest section and, where possible, an
# adjacent-version zstd patch. This script deliberately only reads the stable
# manifest: it never changes it or publishes release assets.
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=release-semver.sh
source "$script_dir/release-semver.sh"
# shellcheck source=release-stable-migration.sh
source "$script_dir/release-stable-migration.sh"

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
delta_manifest="${DELTA_CLI_MANIFEST:-client/src-tauri/Cargo.toml}"
updater_public_key="${TAURI_UPDATER_PUBLIC_KEY:?TAURI_UPDATER_PUBLIC_KEY is required}"

test -s "$manifest"
test -s "$portable_file"
test -s "$nsis_file"
command -v jq >/dev/null
command -v curl >/dev/null
command -v sha256sum >/dev/null

if [[ "$tag" == *-* || "$version" == *-* ]]; then
  echo "Prerelease client builds are standalone test packages and cannot update the stable manifest." >&2
  exit 1
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

file_size() { wc -c <"$1" | tr -d '[:space:]'; }
file_hash() { sha256sum "$1" | awk '{print $1}'; }
file_url() { printf '%s/%s' "${base_url%/}" "$(basename "$1")"; }
sign_file() {
  # tauri signer reads the private key and its password from the two CI secrets.
  local file="$1"
  local absolute_file
  absolute_file="$(cd "$(dirname "$file")" && pwd)/$(basename "$file")"
  (cd "$client_dir" && npx --no-install tauri signer sign "$absolute_file" >/dev/null)
  cargo run --quiet --manifest-path "$delta_manifest" --bin bb-erp-client-delta -- \
    verify-signature --public-key "$updater_public_key" --file "$absolute_file" --signature-file "$absolute_file.sig" >/dev/null || return 1
  local signature
  signature="$(tr -d '\r\n' <"$absolute_file.sig")"
  rm -f "$absolute_file.sig"
  [[ "$signature" =~ ^[A-Za-z0-9+/=]+$ ]] || { echo "Tauri signer returned an invalid signature for $file" >&2; return 1; }
  printf '%s' "$signature" | base64 --decode >/dev/null 2>&1 || {
    echo "Tauri signer returned a non-decodable signature for $file" >&2; return 1;
  }
  printf '%s' "$signature"
}

portable_hash="$(file_hash "$portable_file")"
portable_size="$(file_size "$portable_file")"
nsis_hash="$(file_hash "$nsis_file")"
nsis_size="$(file_size "$nsis_file")"
portable_signature="$(sign_file "$portable_file")"
nsis_signature="$(sign_file "$nsis_file")"
test -n "$portable_signature"
test -n "$nsis_signature"

deltas='[]'
previous_manifest="$work_dir/previous-manifest.json"
stable_http_code="$(curl --silent --show-error --location --retry 3 --retry-all-errors \
  --output "$previous_manifest" --write-out '%{http_code}' "$stable_manifest_url" || true)"
if [[ "$stable_http_code" == "200" ]]; then
  stable_version="$(jq -er '.version' "$previous_manifest")" || {
    echo "Stable manifest has no valid version; refusing to publish without downgrade protection." >&2
    exit 1
  }
  version_comparison="$(semver_compare "$version" "$stable_version")" || {
    echo "Current or stable manifest version is not valid SemVer: current=$version stable=$stable_version" >&2
    exit 1
  }
  if [[ "$version_comparison" != "1" ]] \
    && ! release_allows_historical_stable_migration "$stable_version" "$version"; then
    echo "Release version must be greater than stable version: current=$version stable=$stable_version" >&2
    exit 1
  fi
  if [[ "$version_comparison" != "1" ]]; then
    echo "Using the explicitly authorized historical stable migration: $stable_version -> $version."
  fi
  previous_payload_b64="$(jq -er '.client_update_v2.payload // empty' "$previous_manifest" || true)"
  if [[ -n "$previous_payload_b64" ]]; then
    previous_payload="$work_dir/previous-payload.json"
    previous_payload_signature="$work_dir/previous-payload.sig"
    jq -er '.client_update_v2.signature // empty' "$previous_manifest" >"$previous_payload_signature" 2>/dev/null || true
    if printf '%s' "$previous_payload_b64" | base64 --decode >"$previous_payload" 2>/dev/null \
      && [[ -s "$previous_payload_signature" ]] \
      && cargo run --quiet --manifest-path "$delta_manifest" --bin bb-erp-client-delta -- \
        verify-signature --public-key "$updater_public_key" --file "$previous_payload" --signature-file "$previous_payload_signature" >/dev/null \
      && jq -e --argjson layout "$layout_version" \
        '.protocol_version == 2 and .target == "windows-x86_64" and .layout_version == $layout' \
        "$previous_payload" >/dev/null; then
      previous_version="$(jq -er '.version' "$previous_payload")"
      current_mm="$(sed -E 's/^([0-9]+\.[0-9]+).*/\1/' <<<"$version")"
      previous_mm="$(sed -E 's/^([0-9]+\.[0-9]+).*/\1/' <<<"$previous_version")"
      previous_url="$(jq -er '.full.portable.url' "$previous_payload")"
      previous_hash="$(jq -er '.full.portable.sha256' "$previous_payload")"
      if [[ "$current_mm" == "$previous_mm" && "$previous_version" != "$version" ]]; then
        previous_exe="$work_dir/previous-client.exe"
        if curl --fail --silent --show-error --location --retry 3 --retry-all-errors \
          "$previous_url" -o "$previous_exe" \
          && [[ "$(file_hash "$previous_exe")" == "$previous_hash" ]]; then
          patch_file="$asset_dir/bb-erp-client-windows-x86_64-${previous_version}-to-${version}.zstpatch"
          rebuilt_file="$work_dir/rebuilt-client.exe"
          cargo run --quiet --manifest-path "$delta_manifest" --bin bb-erp-client-delta -- \
            create --old "$previous_exe" --new "$portable_file" --output "$patch_file"
          cargo run --quiet --manifest-path "$delta_manifest" --bin bb-erp-client-delta -- \
            verify --old "$previous_exe" --patch "$patch_file" --output "$rebuilt_file" \
            --expected-sha256 "$portable_hash"
          patch_size="$(file_size "$patch_file")"
          # A patch which is not materially smaller than the signed installer is
          # deliberately omitted; clients then select a signed full update.
          if (( patch_size * 100 < nsis_size * 80 )); then
            patch_hash="$(file_hash "$patch_file")"
            patch_signature="$(sign_file "$patch_file")"
            patch_url="$(file_url "$patch_file")"
            deltas="$(jq -cn \
              --arg from_version "$previous_version" --arg from_sha256 "$previous_hash" \
              --arg target_sha256 "$portable_hash" --arg algorithm "zstd-patch-from-v1" \
              --arg url "$patch_url" --arg signature "$patch_signature" \
              --argjson size "$patch_size" \
              '[{kind:"delta",from_version:$from_version,from_sha256:$from_sha256,target_sha256:$target_sha256,algorithm:$algorithm,url:$url,size:$size,sha256:$target_sha256,signature:$signature}]' \
              | jq --arg sha "$patch_hash" '.[0].sha256 = $sha')"
            echo "Generated client delta $previous_version -> $version."
          else
            rm -f "$patch_file"
            echo "Client delta is at least 80% of the NSIS installer; using full update only."
          fi
        else
          echo "Previous portable client could not be downloaded and verified; using full update only." >&2
        fi
      else
        echo "Previous release is not in the same major.minor line; using full update only."
      fi
    else
      echo "Previous stable manifest v2 payload is invalid, unsigned, or incompatible; using full update only." >&2
    fi
  else
    echo "Previous stable manifest has no v2 payload; this is a full-update baseline."
  fi
elif [[ "$stable_http_code" == "404" ]]; then
  echo "No stable manifest is available yet; this is a full-update baseline."
else
  echo "Unable to read stable manifest (HTTP ${stable_http_code:-000}); refusing to publish without downgrade protection." >&2
  exit 1
fi

payload_file="$work_dir/client-update-v2-payload.json"
jq -cn \
  --arg version "$version" --arg target "windows-x86_64" --argjson layout "$layout_version" \
  --arg nsis_url "$(file_url "$nsis_file")" --arg nsis_sha "$nsis_hash" --arg nsis_signature "$nsis_signature" --argjson nsis_size "$nsis_size" \
  --arg portable_url "$(file_url "$portable_file")" --arg portable_sha "$portable_hash" --arg portable_signature "$portable_signature" --argjson portable_size "$portable_size" \
  --argjson deltas "$deltas" \
  '{protocol_version:2,version:$version,target:$target,layout_version:$layout,full:{nsis:{kind:"nsis",url:$nsis_url,size:$nsis_size,sha256:$nsis_sha,signature:$nsis_signature},portable:{kind:"portable",url:$portable_url,size:$portable_size,sha256:$portable_sha,signature:$portable_signature}},deltas:$deltas}' \
  >"$payload_file"
payload_b64="$(base64 <"$payload_file" | tr -d '\r\n')"
payload_signature="$(sign_file "$payload_file")"
test -n "$payload_signature"

tmp_manifest="$work_dir/update-manifest.json"
jq --arg payload "$payload_b64" --arg signature "$payload_signature" \
  '.client_update_v2 = {payload:$payload, signature:$signature}' \
  "$manifest" >"$tmp_manifest"
mv "$tmp_manifest" "$manifest"

echo "Signed client_update_v2 manifest prepared for $tag."
