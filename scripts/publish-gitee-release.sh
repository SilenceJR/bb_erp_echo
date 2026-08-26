#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=release-semver.sh
source "$script_dir/release-semver.sh"

api_base="${GITEE_API_BASE:-https://gitee.com/api/v5}"
web_base="${GITEE_WEB_BASE:-https://gitee.com}"
tag="${RELEASE_TAG:?RELEASE_TAG is required}"
source_owner="${GITEE_SOURCE_OWNER:?GITEE_SOURCE_OWNER is required}"
source_repo="${GITEE_SOURCE_REPO:?GITEE_SOURCE_REPO is required}"
release_owner="${GITEE_RELEASE_OWNER:?GITEE_RELEASE_OWNER is required}"
release_repo="${GITEE_RELEASE_REPO:?GITEE_RELEASE_REPO is required}"
token="${GITEE_TOKEN:?GITEE_TOKEN is required}"
source_token="${GITEE_SOURCE_TOKEN:-$token}"
expected_sha="${GITHUB_SHA:?GITHUB_SHA is required}"
asset_dir="${RELEASE_ASSET_DIR:-release-assets}"
stable_url="$web_base/$release_owner/$release_repo/raw/main/update-manifest.json"

if [[ ! "$tag" =~ ^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$ ]]; then
  echo "Invalid release tag: $tag" >&2
  exit 1
fi
if [[ "$tag" == *-* ]]; then
  prerelease_part="${tag#*-}"
  IFS='.' read -r -a prerelease_ids <<<"$prerelease_part"
  for identifier in "${prerelease_ids[@]}"; do
    if [[ "$identifier" =~ ^[0-9]+$ && ${#identifier} -gt 1 && "$identifier" == 0* ]]; then
      echo "Invalid numeric prerelease identifier with a leading zero: $identifier" >&2
      exit 1
    fi
  done
fi

if [[ ! -s "$asset_dir/update-manifest.json" ]]; then
  nested_manifest="$(find "$asset_dir" -mindepth 2 -maxdepth 2 -name update-manifest.json -type f -print -quit)"
  if [[ -n "$nested_manifest" ]]; then
    asset_dir="$(dirname "$nested_manifest")"
  fi
fi
manifest="$asset_dir/update-manifest.json"
test -s "$manifest" || { echo "Missing release manifest: $manifest" >&2; exit 1; }
if jq -e '.client_update_v2? != null' "$manifest" >/dev/null; then
  payload_signature="$(jq -er '.client_update_v2.signature' "$manifest")"
  printf '%s' "$payload_signature" | base64 --decode >/dev/null 2>&1 || {
    echo "Invalid client_update_v2 payload signature." >&2; exit 1;
  }
fi

# Keep the legacy ZIP resources and the v2 portable/NSIS/delta resources in one
# manifest-driven collection. This prevents an added update resource from being
# released without the final anonymous checksum verification.
declare -A resource_hashes resource_sizes
while IFS=$'\t' read -r url sha size signature; do
  [[ -n "$url" && -n "$sha" && -n "$size" ]] || { echo "Malformed resource in $manifest" >&2; exit 1; }
  file="${url##*/}"
  expected_url="$web_base/$release_owner/$release_repo/releases/download/$tag/$file"
  [[ "$url" == "$expected_url" ]] || { echo "Release resource URL is not for this tag: $url" >&2; exit 1; }
  [[ "$file" != */* && "$file" != "." && "$file" != ".." ]] || { echo "Invalid release asset name: $file" >&2; exit 1; }
  test -s "$asset_dir/$file" || { echo "Missing release asset: $asset_dir/$file" >&2; exit 1; }
  if [[ -n "$signature" ]]; then
    printf '%s' "$signature" | base64 --decode >/dev/null 2>&1 || {
      echo "Invalid update signature for $file" >&2; exit 1;
    }
  fi
  if [[ -n "${resource_hashes[$file]+present}" ]]; then
    [[ "${resource_hashes[$file]}" == "$sha" && "${resource_sizes[$file]}" == "$size" ]] || {
      echo "Conflicting metadata for release asset: $file" >&2; exit 1;
    }
  else
    resource_hashes[$file]="$sha"
    resource_sizes[$file]="$size"
  fi
done < <(jq -er '
  def resource:
    select(type == "object" and (.url | type) == "string" and (.sha256 | type) == "string" and (.size | type) == "number")
    | [.url, .sha256, .size, (.signature? // "")] | @tsv;
  def signed_resource:
    if type == "object" and (.url | type) == "string" and (.sha256 | type) == "string" and (.size | type) == "number"
      and (.signature | type) == "string" and (.signature | length) > 0
      then [.url, .sha256, .size, .signature] | @tsv
      else error("client_update_v2 resource metadata or signature is missing") end;
  [.server?, .client?, .all_in_one?, .updater? | select(. != null) | resource][],
  (if .client_update_v2? == null then empty else
    (.client_update_v2.payload | @base64d | fromjson) as $payload
    | if $payload.protocol_version != 2 then error("unsupported client_update_v2 protocol") else . end
    | if (.client_update_v2.signature | type) != "string" or (.client_update_v2.signature | length) == 0
      then error("client_update_v2 payload signature is missing") else . end
    | if (.client_update_v2.signature | test("^[A-Za-z0-9+/=]+$"))
      then . else error("client_update_v2 payload signature is invalid") end
    | [$payload.full.nsis, $payload.full.portable | signed_resource][],
      ($payload.deltas | if type == "array" then . else error("client_update_v2 deltas must be an array") end | .[] | signed_resource)
  end)' "$manifest")
(( ${#resource_hashes[@]} > 0 )) || { echo "Manifest contains no publishable resources." >&2; exit 1; }

auth=(-H "Authorization: Bearer $token")
source_auth=(-H "Authorization: Bearer $source_token")
json=(-H "Accept: application/json")

echo "Verifying that the Gitee source tag matches the mirrored GitHub commit..."
source_tags="$(curl --fail --silent --show-error --location "${source_auth[@]}" "${json[@]}" \
  "$api_base/repos/$source_owner/$source_repo/tags?sort=updated&direction=desc&per_page=100")"
source_sha="$(jq -r --arg tag "$tag" '.[] | select(.name == $tag) | .commit.sha' <<<"$source_tags" | head -n1)"
if [[ -z "$source_sha" || "$source_sha" == "null" ]]; then
  echo "The source tag $tag was not found on Gitee. Wait for mirror synchronization and retry." >&2
  exit 1
fi
if [[ "$source_sha" != "$expected_sha" ]]; then
  echo "Tag commit mismatch: Gitee=$source_sha GitHub=$expected_sha" >&2
  exit 1
fi

release_url="$api_base/repos/$release_owner/$release_repo/releases/tags/$tag"
release_lookup="$(curl --fail --silent --show-error --location "${auth[@]}" "${json[@]}" "$release_url")"
if jq -e 'type == "object" and .id != null' <<<"$release_lookup" >/dev/null; then
  echo "Release $tag already exists in the Gitee distribution repository; refusing to overwrite it." >&2
  exit 1
fi
if ! jq -e '. == null' <<<"$release_lookup" >/dev/null; then
  echo "Unexpected response while checking Release $tag; refusing to publish." >&2
  exit 1
fi

distribution_main="$(curl --fail --silent --show-error --location "${auth[@]}" "${json[@]}" \
  "$api_base/repos/$release_owner/$release_repo/branches/main")"
distribution_main_sha="$(jq -er '.commit.sha' <<<"$distribution_main")"
distribution_tags="$(curl --fail --silent --show-error --location "${auth[@]}" "${json[@]}" \
  "$api_base/repos/$release_owner/$release_repo/tags?sort=updated&direction=desc&per_page=100")"
distribution_tag_sha="$(jq -r --arg tag "$tag" '.[] | select(.name == $tag) | .commit.sha' <<<"$distribution_tags" | head -n1)"
if [[ -z "$distribution_tag_sha" || "$distribution_tag_sha" == "null" ]]; then
  echo "Creating the Gitee distribution tag..."
  curl --fail --silent --show-error --location -X POST "${auth[@]}" "${json[@]}" \
    --data-urlencode "tag_name=$tag" \
    --data-urlencode "refs=main" \
    "$api_base/repos/$release_owner/$release_repo/tags" >/dev/null
elif [[ "$distribution_tag_sha" == "$distribution_main_sha" ]]; then
  echo "Reusing existing distribution tag $tag at the current main commit."
else
  echo "Distribution tag $tag points to $distribution_tag_sha, not current main $distribution_main_sha; refusing to reuse it." >&2
  exit 1
fi

prerelease=false
[[ "$tag" == *-* ]] && prerelease=true
echo "Creating the Gitee distribution release..."
release_json="$(curl --fail --silent --show-error --location -X POST "${auth[@]}" "${json[@]}" \
  --data-urlencode "tag_name=$tag" \
  --data-urlencode "name=BB ERP $tag" \
  --data-urlencode "body=Automated Windows release for $tag" \
  --data-urlencode "target_commitish=main" \
  --data-urlencode "prerelease=$prerelease" \
  "$api_base/repos/$release_owner/$release_repo/releases")"
release_id="$(jq -er '.id' <<<"$release_json")"

echo "Uploading versioned release assets..."
for file in "${!resource_hashes[@]}"; do
  curl --fail --silent --show-error --location -X POST "${auth[@]}" "${json[@]}" \
    -F "file=@$asset_dir/$file" \
    "$api_base/repos/$release_owner/$release_repo/releases/$release_id/attach_files" >/dev/null
done

echo "Verifying anonymous downloads, sizes, and SHA-256 hashes..."
verify_dir="$(mktemp -d)"
trap 'rm -rf "$verify_dir"' EXIT
for file in "${!resource_hashes[@]}"; do
  url="$web_base/$release_owner/$release_repo/releases/download/$tag/$file"
  curl --fail --silent --show-error --location --retry 6 --retry-all-errors \
    --output "$verify_dir/$file" "$url"
  expected_hash="${resource_hashes[$file]}"
  expected_size="${resource_sizes[$file]}"
  actual_hash="$(sha256sum "$verify_dir/$file" | awk '{print $1}')"
  actual_size="$(stat -c '%s' "$verify_dir/$file")"
  [[ "$actual_hash" == "$expected_hash" ]] || { echo "SHA-256 mismatch for $file" >&2; exit 1; }
  [[ "$actual_size" == "$expected_size" ]] || { echo "Size mismatch for $file" >&2; exit 1; }
done

echo "Rechecking stable version immediately before publishing the manifest..."
current_stable="$verify_dir/current-stable.json"
stable_code="$(curl --silent --show-error --location --retry 3 --retry-all-errors \
  --output "$current_stable" --write-out '%{http_code}' "$stable_url?candidate=$tag" || true)"
release_version="$(jq -er '.version' "$manifest")"
if [[ "$stable_code" == "200" ]]; then
  current_stable_version="$(jq -er '.version' "$current_stable")" || {
    echo "Current stable manifest has no valid version; refusing to overwrite it." >&2
    exit 1
  }
  stable_comparison="$(semver_compare "$release_version" "$current_stable_version")" || {
    echo "Release or current stable version is not valid SemVer: release=$release_version stable=$current_stable_version" >&2
    exit 1
  }
  if [[ "$stable_comparison" != "1" ]]; then
    echo "Release version must be greater than current stable version: release=$release_version stable=$current_stable_version" >&2
    exit 1
  fi
elif [[ "$stable_code" != "404" ]]; then
  echo "Unable to recheck stable manifest (HTTP ${stable_code:-000}); refusing to update it." >&2
  exit 1
fi
bash "$script_dir/check-release-delta-base.sh" "$manifest" "$current_stable"

echo "Publishing the stable manifest only after all assets passed verification..."
content_api="$api_base/repos/$release_owner/$release_repo/contents/update-manifest.json"
existing_response="$verify_dir/existing.json"
existing_code="$(curl --silent --show-error --location "${auth[@]}" "${json[@]}" \
  --output "$existing_response" --write-out '%{http_code}' "$content_api?ref=main")"
encoded_content="$(base64 <"$manifest" | tr -d '\n')"
if [[ "$existing_code" == "200" ]] && jq -e 'type == "object" and (.sha | type == "string")' "$existing_response" >/dev/null; then
  existing_sha="$(jq -er '.sha' "$existing_response")"
  payload="$(jq -n --arg content "$encoded_content" --arg sha "$existing_sha" --arg message "chore: publish $tag manifest" \
    '{content:$content, sha:$sha, message:$message, branch:"main"}')"
  method=PUT
elif [[ "$existing_code" == "404" ]] \
  || { [[ "$existing_code" == "200" ]] && jq -e 'type == "array" and length == 0' "$existing_response" >/dev/null; }; then
  payload="$(jq -n --arg content "$encoded_content" --arg message "chore: publish $tag manifest" \
    '{content:$content, message:$message, branch:"main"}')"
  method=POST
else
  echo "Unable to inspect stable manifest (HTTP $existing_code)." >&2
  exit 1
fi
curl --fail --silent --show-error --location -X "$method" "${auth[@]}" "${json[@]}" \
  -H "Content-Type: application/json" --data "$payload" "$content_api" >/dev/null

for attempt in {1..12}; do
  if curl --fail --silent --show-error --location \
    "$stable_url?release=$tag&attempt=$attempt" -o "$verify_dir/stable.json" \
    && cmp --silent "$manifest" "$verify_dir/stable.json"; then
    echo "Published and verified: $stable_url"
    exit 0
  fi
  sleep 10
done
echo "Stable manifest did not become anonymously readable in time." >&2
exit 1
