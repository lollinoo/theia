#!/usr/bin/env bash
set -euo pipefail

# Waits for PostgreSQL and runs the Playwright browser E2E gate.

: "${THEIA_E2E_DB_DSN:?THEIA_E2E_DB_DSN is required}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"

# shellcheck source=scripts/ci/lib.sh
source "$script_dir/lib.sh"

cd "$repo_dir"

wait_for_postgres "$THEIA_E2E_DB_DSN"
make browser-e2e
