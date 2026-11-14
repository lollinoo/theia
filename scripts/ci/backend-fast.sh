#!/usr/bin/env bash
set -euo pipefail

# Runs PostgreSQL repository regressions and the backend-fast Makefile quality gate.

: "${THEIA_TEST_DB_DSN:?THEIA_TEST_DB_DSN is required}"

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_dir="$(cd "$script_dir/../.." && pwd)"

# shellcheck source=scripts/ci/lib.sh
source "$script_dir/lib.sh"

cd "$repo_dir"

gopath_bin="$(go env GOPATH)/bin"
export PATH="$PATH:$gopath_bin"

if ! command -v govulncheck >/dev/null 2>&1; then
  go install golang.org/x/vuln/cmd/govulncheck@latest
fi

wait_for_postgres "$THEIA_TEST_DB_DSN"

go test ./internal/repository/postgres -run '^(TestSettingsRepoUpdateSerializesConcurrentMutations|TestAuthRepoListUsersPostgresBatchAggregates)$' -count=1

# Keep the full backend-fast suite running without THEIA_TEST_DB_DSN, matching historic behavior.
env -u THEIA_TEST_DB_DSN make backend-fast
