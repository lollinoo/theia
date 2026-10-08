#!/usr/bin/env bash
set -euo pipefail

# Compile on the development host and run in the runtime image. Tests create
# isolated PostgreSQL clusters and never touch the deployed instance.
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
cd "$repo_dir"
docker build --target production -t theia-maintenance-test:local .
go test -c -o "$test_dir/service.test" ./internal/service
docker run --rm \
  -e THEIA_MAINTENANCE_INTEGRATION=1 \
  -v "$test_dir/service.test:/service.test:ro" \
  theia-maintenance-test:local \
  /service.test -test.run '^Test(ManagedBackup|ManagedActivation|Maintenance)Integration$' -test.v -test.timeout 240s
