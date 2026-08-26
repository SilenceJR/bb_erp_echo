#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT

make_manifest() {
  local output="$1" version="$2" portable_sha="$3" deltas="$4" payload payload_b64
  payload="$(jq -cn --arg version "$version" --arg sha "$portable_sha" --argjson deltas "$deltas" \
    '{protocol_version:2,version:$version,target:"windows-x86_64",layout_version:1,full:{portable:{sha256:$sha}},deltas:$deltas}')"
  payload_b64="$(printf '%s' "$payload" | base64 | tr -d '\r\n')"
  jq -n --arg version "$version" --arg payload "$payload_b64" \
    '{version:$version,client_update_v2:{payload:$payload,signature:"test"}}' >"$output"
}

make_manifest "$work_dir/stable.json" 1.0.0-rc.4 stable-sha '[]'
make_manifest "$work_dir/valid.json" 1.0.0-rc.5 new-sha '[{"from_version":"1.0.0-rc.4","from_sha256":"stable-sha"}]'
bash "$script_dir/check-release-delta-base.sh" "$work_dir/valid.json" "$work_dir/stable.json" >/dev/null

make_manifest "$work_dir/wrong-version.json" 1.0.0-rc.5 new-sha '[{"from_version":"1.0.0-rc.3","from_sha256":"stable-sha"}]'
if bash "$script_dir/check-release-delta-base.sh" "$work_dir/wrong-version.json" "$work_dir/stable.json" >/dev/null 2>&1; then
  echo "Accepted a delta built from a stale version." >&2
  exit 1
fi
make_manifest "$work_dir/wrong-sha.json" 1.0.0-rc.5 new-sha '[{"from_version":"1.0.0-rc.4","from_sha256":"old-sha"}]'
if bash "$script_dir/check-release-delta-base.sh" "$work_dir/wrong-sha.json" "$work_dir/stable.json" >/dev/null 2>&1; then
  echo "Accepted a delta built from a stale portable executable." >&2
  exit 1
fi
make_manifest "$work_dir/multiple.json" 1.0.0-rc.5 new-sha '[{"from_version":"1.0.0-rc.4","from_sha256":"stable-sha"},{"from_version":"1.0.0-rc.3","from_sha256":"older-sha"}]'
if bash "$script_dir/check-release-delta-base.sh" "$work_dir/multiple.json" "$work_dir/stable.json" >/dev/null 2>&1; then
  echo "Accepted multiple candidate deltas." >&2
  exit 1
fi

echo "Release delta base checks passed."
