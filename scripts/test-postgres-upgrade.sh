#!/usr/bin/env bash
set -euo pipefail
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"
docker build --pull --target production -t theia-managed-backend:local .
docker build --pull --target production -f Dockerfile.frontend -t theia-managed-frontend:local frontend
docker pull postgres:17-bookworm
docker pull postgres:18-bookworm
THEIA_DEPLOYMENT_INTEGRATION=1 go test ./internal/deployment -run '^Test(PostgresUpgrade|LegacyImport)Integration$' -v -count=1 -timeout 12m
