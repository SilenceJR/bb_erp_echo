#!/usr/bin/env bash
set -euo pipefail

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

required=(
  bb-erp-server-windows.zip
  bb-erp-client-windows.zip
  bb-erp-all-in-one-windows.zip
  bb-erp-updater-windows.zip
  update-manifest.json
)
for file in "${required[@]}"; do
  test -s "$asset_dir/$file" || { echo "Missing release asset: $asset_dir/$file" >&2; exit 1; }
done

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

echo "Creating the Gitee distribution tag and release..."
curl --fail --silent --show-error --location -X POST "${auth[@]}" "${json[@]}" \
  --data-urlencode "tag_name=$tag" \
  --data-urlencode "refs=main" \
  "$api_base/repos/$release_owner/$release_repo/tags" >/dev/null

prerelease=false
[[ "$tag" == *-* ]] && prerelease=true
release_json="$(curl --fail --silent --show-error --location -X POST "${auth[@]}" "${json[@]}" \
  --data-urlencode "tag_name=$tag" \
  --data-urlencode "name=BB ERP $tag" \
  --data-urlencode "body=Automated Windows release for $tag" \
  --data-urlencode "target_commitish=main" \
  --data-urlencode "prerelease=$prerelease" \
  "$api_base/repos/$release_owner/$release_repo/releases")"
release_id="$(jq -er '.id' <<<"$release_json")"

echo "Uploading versioned release assets..."
for file in "${required[@]:0:4}"; do
  curl --fail --silent --show-error --location -X POST "${auth[@]}" "${json[@]}" \
    -F "file=@$asset_dir/$file" \
    "$api_base/repos/$release_owner/$release_repo/releases/$release_id/attach_files" >/dev/null
done

echo "Verifying anonymous downloads, sizes, and SHA-256 hashes..."
verify_dir="$(mktemp -d)"
trap 'rm -rf "$verify_dir"' EXIT
for kind in server client all_in_one updater; do
  case "$kind" in
    server) file=bb-erp-server-windows.zip ;;
    client) file=bb-erp-client-windows.zip ;;
    all_in_one) file=bb-erp-all-in-one-windows.zip ;;
    updater) file=bb-erp-updater-windows.zip ;;
  esac
  url="$web_base/$release_owner/$release_repo/releases/download/$tag/$file"
  curl --fail --silent --show-error --location --retry 6 --retry-all-errors \
    --output "$verify_dir/$file" "$url"
  expected_hash="$(jq -er ".$kind.sha256" "$asset_dir/update-manifest.json")"
  expected_size="$(jq -er ".$kind.size" "$asset_dir/update-manifest.json")"
  actual_hash="$(sha256sum "$verify_dir/$file" | awk '{print $1}')"
  actual_size="$(stat -c '%s' "$verify_dir/$file")"
  [[ "$actual_hash" == "$expected_hash" ]] || { echo "SHA-256 mismatch for $file" >&2; exit 1; }
  [[ "$actual_size" == "$expected_size" ]] || { echo "Size mismatch for $file" >&2; exit 1; }
done

echo "Publishing the stable manifest only after all assets passed verification..."
content_api="$api_base/repos/$release_owner/$release_repo/contents/update-manifest.json"
existing_response="$verify_dir/existing.json"
existing_code="$(curl --silent --show-error --location "${auth[@]}" "${json[@]}" \
  --output "$existing_response" --write-out '%{http_code}' "$content_api?ref=main")"
encoded_content="$(base64 <"$asset_dir/update-manifest.json" | tr -d '\n')"
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

stable_url="$web_base/$release_owner/$release_repo/raw/main/update-manifest.json"
for attempt in {1..12}; do
  if curl --fail --silent --show-error --location \
    "$stable_url?release=$tag&attempt=$attempt" -o "$verify_dir/stable.json" \
    && cmp --silent "$asset_dir/update-manifest.json" "$verify_dir/stable.json"; then
    echo "Published and verified: $stable_url"
    exit 0
  fi
  sleep 10
done
echo "Stable manifest did not become anonymously readable in time." >&2
exit 1
