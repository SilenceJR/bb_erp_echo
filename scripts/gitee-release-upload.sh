#!/usr/bin/env bash
# Resumable Gitee Release attachment upload helpers.
# The caller provides api_base, release_owner, release_repo, release_id,
# token, asset_dir, auth and json.

gitee_release_assets_url() {
  printf '%s/repos/%s/%s/releases/%s/attach_files' \
    "$api_base" "$release_owner" "$release_repo" "$release_id"
}

gitee_list_release_assets() {
  curl --fail --silent --show-error --location "${auth[@]}" "${json[@]}" \
    "$(gitee_release_assets_url)"
}

gitee_release_asset_status() {
  local file="$1" expected_size="$2" response
  if ! response="$(gitee_list_release_assets)"; then
    echo "Unable to list Gitee Release attachments for $file." >&2
    return 2
  fi
  jq -er --arg file "$file" --argjson expected_size "$expected_size" '
    def items:
      if . == null then []
      elif type == "array" then .
      elif (.data? | type) == "array" then .data
      elif (.assets? | type) == "array" then .assets
      else error("unexpected Gitee attachment list response") end;
    [items[] | select((.name? // .filename? // .file_name? // "") == $file)] as $matches
    | if ($matches | length) == 0 then "absent"
      elif any($matches[];
        ((.size? // .file_size? // .filesize?) == null)
        or (((.size? // .file_size? // .filesize?) | tonumber?) == $expected_size))
      then "present"
      else "conflict" end
  ' <<<"$response"
}

gitee_wait_for_release_asset() {
  local file="$1" expected_size="$2" checks="${3:-3}" delay="${GITEE_UPLOAD_CONFIRM_DELAY:-5}"
  local check status
  for ((check = 1; check <= checks; check++)); do
    status="$(gitee_release_asset_status "$file" "$expected_size")" || return $?
    case "$status" in
      present) return 0 ;;
      conflict)
        echo "Gitee Release already contains a conflicting attachment named $file." >&2
        return 2
        ;;
    esac
    (( check < checks )) && sleep "$delay"
  done
  return 1
}

gitee_upload_release_asset() {
  local file="$1" expected_size="$2"
  local attempts="${GITEE_UPLOAD_ATTEMPTS:-3}"
  local max_time="${GITEE_UPLOAD_MAX_TIME:-1800}"
  local speed_limit="${GITEE_UPLOAD_SPEED_LIMIT:-1024}"
  local speed_time="${GITEE_UPLOAD_SPEED_TIME:-120}"
  local retry_delay="${GITEE_UPLOAD_RETRY_DELAY:-10}"
  local attempt metrics curl_status response_file

  if gitee_wait_for_release_asset "$file" "$expected_size" 1; then
    echo "Reusing existing Gitee Release attachment: $file"
    return 0
  else
    case "$?" in
      1) ;;
      *) return 1 ;;
    esac
  fi

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    response_file="$(mktemp)"
    metrics=""
    curl_status=0
    if metrics="$(curl --fail --silent --show-error --location --http1.1 -X POST \
      "${json[@]}" -H "Expect:" \
      --connect-timeout 30 --max-time "$max_time" \
      --speed-limit "$speed_limit" --speed-time "$speed_time" \
      -F "access_token=$token" \
      -F "owner=$release_owner" \
      -F "repo=$release_repo" \
      -F "release_id=$release_id" \
      -F "file=@$asset_dir/$file" \
      --output "$response_file" \
      --write-out 'http=%{http_code} uploaded=%{size_upload} total=%{time_total}' \
      "$(gitee_release_assets_url)")"; then
      curl_status=0
    else
      curl_status=$?
    fi
    rm -f "$response_file"
    echo "Gitee upload $file attempt $attempt/$attempts: exit=$curl_status ${metrics:-no-metrics}"

    if gitee_wait_for_release_asset "$file" "$expected_size" 3; then
      echo "Confirmed Gitee Release attachment: $file"
      return 0
    else
      case "$?" in
        1) ;;
        *) return 1 ;;
      esac
    fi
    (( attempt < attempts )) && sleep "$retry_delay"
  done

  echo "Failed to upload and confirm Gitee Release attachment after $attempts attempts: $file" >&2
  return 1
}
