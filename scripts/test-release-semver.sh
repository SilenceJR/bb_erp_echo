#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=release-semver.sh
source "$script_dir/release-semver.sh"

assert_compare() {
  local expected="$1" left="$2" right="$3" actual
  actual="$(semver_compare "$left" "$right")"
  [[ "$actual" == "$expected" ]] || {
    echo "SemVer comparison failed: $left vs $right, expected $expected, got $actual" >&2
    exit 1
  }
}

assert_compare 1 1.0.0 0.9.9
assert_compare 1 1.0.0 1.0.0-rc.9
assert_compare 1 1.0.0-rc.10 1.0.0-rc.9
assert_compare 1 1.0.0-rc.1.1 1.0.0-rc.1
assert_compare -1 1.0.0-alpha 1.0.0-alpha.1
assert_compare -1 1.0.0-1 1.0.0-alpha
assert_compare 0 v1.2.3+build.2 1.2.3+build.1
assert_compare -1 1.2.3 2.0.0
if semver_compare 01.0.0 1.0.0 >/dev/null 2>&1; then
  echo "Invalid SemVer with a leading zero was accepted." >&2
  exit 1
fi
if semver_compare 1.0.0-01 1.0.0-1 >/dev/null 2>&1; then
  echo "Invalid numeric prerelease identifier with a leading zero was accepted." >&2
  exit 1
fi
echo "SemVer release checks passed."
