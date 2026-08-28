#!/usr/bin/env bash
set -euo pipefail
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=release-stable-migration.sh
source "$script_dir/release-stable-migration.sh"

RELEASE_ALLOW_HISTORICAL_STABLE_MIGRATION=true
release_allows_historical_stable_migration 0.1.0-rc.3 0.0.2
release_allows_historical_stable_migration 0.1.0-rc.3 0.0.3
release_allows_historical_stable_migration 0.1.0-rc.3 0.0.4

if release_allows_historical_stable_migration 0.1.0-rc.3 0.0.5; then
  echo "Accepted a historical migration beyond the second fallback release." >&2
  exit 1
fi
if release_allows_historical_stable_migration 0.1.0-rc.4 0.0.2; then
  echo "Accepted an unknown historical stable version." >&2
  exit 1
fi
RELEASE_ALLOW_HISTORICAL_STABLE_MIGRATION=false
if release_allows_historical_stable_migration 0.1.0-rc.3 0.0.2; then
  echo "Accepted a migration without the explicit CI flag." >&2
  exit 1
fi

echo "Historical stable migration checks passed."
