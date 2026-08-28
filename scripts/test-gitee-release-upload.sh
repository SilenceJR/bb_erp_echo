#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=gitee-release-upload.sh
source "$script_dir/gitee-release-upload.sh"

test_root="$(mktemp -d)"
trap 'rm -rf "$test_root"' EXIT
state_file="$test_root/state"
curl_calls="$test_root/curl-calls"
: >"$curl_calls"

api_base=https://gitee.invalid/api/v5
release_owner=test-owner
release_repo=test-repo
release_id=123
token=test-token
asset_dir="$test_root/assets"
mkdir -p "$asset_dir"
auth=(-H "Authorization: Bearer test-token")
json=(-H "Accept: application/json")
GITEE_UPLOAD_ATTEMPTS=2
GITEE_UPLOAD_MAX_TIME=1
GITEE_UPLOAD_RETRY_DELAY=0
GITEE_UPLOAD_CONFIRM_DELAY=0

sleep() { :; }

gitee_list_release_assets() {
  case "$(cat "$state_file" 2>/dev/null || true)" in
    present) printf '[{"name":"asset.zip","size":4}]' ;;
    conflict) printf '[{"name":"asset.zip","size":5}]' ;;
    *) printf '[]' ;;
  esac
}

curl() {
  printf 'call\n' >>"$curl_calls"
  case "${MOCK_CURL_MODE:-success}" in
    success)
      printf 'present' >"$state_file"
      printf 'http=201 uploaded=4 total=0.1'
      return 0
      ;;
    timeout-after-upload)
      printf 'present' >"$state_file"
      printf 'http=000 uploaded=4 total=1.0'
      return 28
      ;;
    fail)
      printf 'http=000 uploaded=0 total=1.0'
      return 28
      ;;
  esac
}

printf data >"$asset_dir/asset.zip"

printf 'present' >"$state_file"
MOCK_CURL_MODE=fail gitee_upload_release_asset asset.zip 4
[[ ! -s "$curl_calls" ]] || { echo "Existing attachment was uploaded again." >&2; exit 1; }

: >"$state_file"
: >"$curl_calls"
MOCK_CURL_MODE=timeout-after-upload gitee_upload_release_asset asset.zip 4
[[ "$(wc -l <"$curl_calls" | tr -d ' ')" == "1" ]] || { echo "Timed-out completed upload was retried." >&2; exit 1; }

printf conflict >"$state_file"
if MOCK_CURL_MODE=success gitee_upload_release_asset asset.zip 4; then
  echo "Conflicting attachment was accepted." >&2
  exit 1
fi

: >"$state_file"
: >"$curl_calls"
if MOCK_CURL_MODE=fail gitee_upload_release_asset asset.zip 4; then
  echo "Repeated upload failure was accepted." >&2
  exit 1
fi
[[ "$(wc -l <"$curl_calls" | tr -d ' ')" == "2" ]] || { echo "Upload attempt limit was not enforced." >&2; exit 1; }

echo "Gitee resumable upload checks passed."
