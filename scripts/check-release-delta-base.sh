#!/usr/bin/env bash
# Ensures a candidate delta was built from the stable release that is current at
# manifest publication time. This closes the gap between parallel build jobs
# and the globally serialized publisher.
set -euo pipefail

candidate_manifest="${1:?candidate manifest is required}"
stable_manifest="${2:?stable manifest is required}"
test -s "$candidate_manifest"

if ! jq -e '.client_update_v2? != null' "$candidate_manifest" >/dev/null; then
  exit 0
fi

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
candidate_payload="$work_dir/candidate.json"
jq -er '.client_update_v2.payload' "$candidate_manifest" | base64 --decode >"$candidate_payload" || {
  echo "Candidate client_update_v2 payload cannot be decoded." >&2
  exit 1
}
delta_count="$(jq -er '.deltas | if type == "array" then length else error("deltas must be an array") end' "$candidate_payload")"
if [[ "$delta_count" == "0" ]]; then
  exit 0
fi
if [[ "$delta_count" != "1" ]]; then
  echo "Candidate release must contain at most one adjacent-version delta; found $delta_count." >&2
  exit 1
fi
test -s "$stable_manifest" || {
  echo "Candidate contains a delta but no current stable manifest is available." >&2
  exit 1
}

stable_payload="$work_dir/stable.json"
jq -er '.client_update_v2.payload' "$stable_manifest" | base64 --decode >"$stable_payload" || {
  echo "Current stable manifest has no decodable client_update_v2 payload for the candidate delta." >&2
  exit 1
}

stable_version="$(jq -er '.version' "$stable_manifest")"
stable_payload_version="$(jq -er '.version' "$stable_payload")"
stable_portable_sha="$(jq -er '.full.portable.sha256' "$stable_payload")"
delta_from_version="$(jq -er '.deltas[0].from_version' "$candidate_payload")"
delta_from_sha="$(jq -er '.deltas[0].from_sha256' "$candidate_payload")"

if [[ "$stable_payload_version" != "$stable_version" ]]; then
  echo "Current stable manifest and signed payload versions disagree." >&2
  exit 1
fi
if [[ "$delta_from_version" != "$stable_version" ]]; then
  echo "Candidate delta source version $delta_from_version is not current stable $stable_version; rebuild the release." >&2
  exit 1
fi
delta_from_sha_lower="$(printf '%s' "$delta_from_sha" | tr '[:upper:]' '[:lower:]')"
stable_portable_sha_lower="$(printf '%s' "$stable_portable_sha" | tr '[:upper:]' '[:lower:]')"
if [[ "$delta_from_sha_lower" != "$stable_portable_sha_lower" ]]; then
  echo "Candidate delta source SHA does not match the current stable portable client; rebuild the release." >&2
  exit 1
fi

echo "Candidate delta base matches current stable $stable_version."
