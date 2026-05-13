#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/wisp-lab-common.sh"

wisp_disconnect_backend_from_lab_network || true
docker compose -f "$WISP_LAB_COMPOSE_FILE" down
