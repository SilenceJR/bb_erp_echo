#!/usr/bin/env bash
# Allows only the explicitly authorized historical stable-manifest migrations.
# The CI flag and exact version pairs keep the normal SemVer downgrade
# protection for every other release.
set -euo pipefail

release_allows_historical_stable_migration() {
  local stable_version="$1" release_version="$2"
  [[ "${RELEASE_ALLOW_HISTORICAL_STABLE_MIGRATION:-false}" == "true" \
    && "$stable_version" == "0.1.0-rc.3" \
    && ("$release_version" == "0.0.2" || "$release_version" == "0.0.3") ]]
}
