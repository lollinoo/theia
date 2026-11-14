#!/usr/bin/env bash
set -euo pipefail

# Maps changed repo-relative paths on stdin (or --all) to backend/frontend/integration flags.

usage() {
  echo "Usage: changes.sh [--all]" >&2
  exit 1
}

all_mode=false
if [[ $# -eq 1 && "$1" == "--all" ]]; then
  all_mode=true
elif [[ $# -ne 0 ]]; then
  usage
fi

backend=false
frontend=false

if [[ "$all_mode" == "true" ]]; then
  backend=true
  frontend=true
else
  saw_path=false
  while IFS= read -r path || [[ -n "$path" ]]; do
    path="${path%$'\r'}"
    [[ -z "$path" ]] && continue
    saw_path=true
    path="${path#./}"

    case "$path" in
      .github/workflows/ci.yml|.woodpecker/*|scripts/ci/*)
        backend=true
        frontend=true
        ;;
    esac

    case "$path" in
      cmd/*|internal/*|go.mod|go.sum|config.example.yaml|Dockerfile|Dockerfile.deployment-test|docker-compose*.yml|Makefile|scripts/*|.env*.example)
        backend=true
        ;;
    esac

    case "$path" in
      frontend/*|Dockerfile.frontend)
        frontend=true
        ;;
    esac
  done

  # Unknown/empty change set must not skip gates.
  if [[ "$saw_path" == "false" ]]; then
    backend=true
    frontend=true
  fi
fi

integration=false
if [[ "$backend" == "true" || "$frontend" == "true" ]]; then
  integration=true
fi

printf 'backend=%s\nfrontend=%s\nintegration=%s\n' "$backend" "$frontend" "$integration"
