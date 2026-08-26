#!/usr/bin/env bash
# Minimal SemVer 2.0.0 comparison used by the release pipeline. Build metadata
# is ignored for precedence, as required by the SemVer specification.
set -euo pipefail
export LC_ALL=C

semver_normalize_number() {
  local value="$1"
  value="${value#"${value%%[!0]*}"}"
  printf '%s' "${value:-0}"
}

semver_compare_number() {
  local left right
  left="$(semver_normalize_number "$1")"
  right="$(semver_normalize_number "$2")"
  if (( ${#left} > ${#right} )); then printf '1'; return; fi
  if (( ${#left} < ${#right} )); then printf '%s' '-1'; return; fi
  if [[ "$left" == "$right" ]]; then printf '0'; return; fi
  if [[ "$left" > "$right" ]]; then printf '1'; else printf '%s' '-1'; fi
}

semver_compare() {
  local left="${1#v}" right="${2#v}"
  local pattern='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-([0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*))?(\+[0-9A-Za-z-]+(\.[0-9A-Za-z-]+)*)?$'
  [[ "$left" =~ $pattern && "$right" =~ $pattern ]] || return 2

  local left_without_build="${left%%+*}" right_without_build="${right%%+*}"
  local left_core="${left_without_build%%-*}" right_core="${right_without_build%%-*}"
  local left_pre="" right_pre=""
  [[ "$left_without_build" == *-* ]] && left_pre="${left_without_build#*-}"
  [[ "$right_without_build" == *-* ]] && right_pre="${right_without_build#*-}"

  local -a left_core_ids right_core_ids left_pre_ids right_pre_ids
  left_pre_ids=()
  right_pre_ids=()
  IFS='.' read -r -a left_core_ids <<<"$left_core"
  IFS='.' read -r -a right_core_ids <<<"$right_core"
  [[ -n "$left_pre" ]] && IFS='.' read -r -a left_pre_ids <<<"$left_pre"
  [[ -n "$right_pre" ]] && IFS='.' read -r -a right_pre_ids <<<"$right_pre"
  local identifier
  if [[ -n "$left_pre" ]]; then
    for identifier in "${left_pre_ids[@]}"; do
      [[ ! "$identifier" =~ ^[0-9]+$ || "$identifier" == "0" || "$identifier" != 0* ]] || return 2
    done
  fi
  if [[ -n "$right_pre" ]]; then
    for identifier in "${right_pre_ids[@]}"; do
      [[ ! "$identifier" =~ ^[0-9]+$ || "$identifier" == "0" || "$identifier" != 0* ]] || return 2
    done
  fi
  local index comparison
  for index in 0 1 2; do
    comparison="$(semver_compare_number "${left_core_ids[$index]}" "${right_core_ids[$index]}")"
    [[ "$comparison" == 0 ]] || { printf '%s' "$comparison"; return; }
  done

  if [[ -z "$left_pre" && -z "$right_pre" ]]; then printf '0'; return; fi
  if [[ -z "$left_pre" ]]; then printf '1'; return; fi
  if [[ -z "$right_pre" ]]; then printf '%s' '-1'; return; fi
  local max_ids="${#left_pre_ids[@]}"
  (( ${#right_pre_ids[@]} > max_ids )) && max_ids="${#right_pre_ids[@]}"
  for ((index = 0; index < max_ids; index++)); do
    if (( index >= ${#left_pre_ids[@]} )); then printf '%s' '-1'; return; fi
    if (( index >= ${#right_pre_ids[@]} )); then printf '1'; return; fi
    local left_id="${left_pre_ids[$index]}" right_id="${right_pre_ids[$index]}"
    [[ "$left_id" == "$right_id" ]] && continue
    if [[ "$left_id" =~ ^[0-9]+$ && "$right_id" =~ ^[0-9]+$ ]]; then
      comparison="$(semver_compare_number "$left_id" "$right_id")"
    elif [[ "$left_id" =~ ^[0-9]+$ ]]; then
      comparison='-1'
    elif [[ "$right_id" =~ ^[0-9]+$ ]]; then
      comparison='1'
    elif [[ "$left_id" > "$right_id" ]]; then
      comparison='1'
    else
      comparison='-1'
    fi
    printf '%s' "$comparison"
    return
  done
  printf '0'
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  [[ "${1:-}" == "compare" && $# == 3 ]] || {
    echo "usage: $0 compare <left-semver> <right-semver>" >&2
    exit 2
  }
  semver_compare "$2" "$3"
  printf '\n'
fi
