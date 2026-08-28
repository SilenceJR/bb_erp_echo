#!/usr/bin/env bash
# Allows exactly one known historical stable-manifest migration. The explicit
# CI flag and both exact versions keep the normal SemVer downgrade protection.
set -euo pipefail

release_allows_historical_stable_migration() {
  local stable_version="$1" release_version="$2"
  [[ "${RELEASE_ALLOW_HISTORICAL_STABLE_MIGRATION:-false}" == "true" \
    && "$stable_version" == "0.1.0-rc.3" \
    && "$release_version" == "0.0.2" ]]
}
