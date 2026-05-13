#!/usr/bin/env bash
set -euo pipefail

WISP_LAB_COMPOSE_FILE="docker-compose.wisp-lab.yml"
WISP_LAB_NETWORK="theia-wisp-lab_wisp-access-mgmt"
WISP_BACKEND_CONTAINER="theia-backend"
WISP_DOCKER_TARGET_PREFIX="172.31.250."
WISP_HOST_TARGET_PREFIX="127.0.10."

wisp_docker_object_exists() {
  local kind="$1"
  local name="$2"
  docker "$kind" inspect "$name" >/dev/null 2>&1
}

wisp_backend_running() {
  [ "$(docker inspect -f '{{.State.Running}}' "$WISP_BACKEND_CONTAINER" 2>/dev/null || true)" = "true" ]
}

wisp_backend_exists() {
  wisp_docker_object_exists container "$WISP_BACKEND_CONTAINER"
}

wisp_container_on_network() {
  local container_name="$1"
  local network_name="$2"
  docker inspect -f '{{range $name, $config := .NetworkSettings.Networks}}{{println $name}}{{end}}' "$container_name" 2>/dev/null | grep -Fxq "$network_name"
}

wisp_connect_backend_to_lab_network() {
  if ! wisp_backend_running; then
    return 1
  fi

  if ! wisp_docker_object_exists network "$WISP_LAB_NETWORK"; then
    echo "WISP lab network '$WISP_LAB_NETWORK' does not exist yet." >&2
    return 1
  fi

  if wisp_container_on_network "$WISP_BACKEND_CONTAINER" "$WISP_LAB_NETWORK"; then
    return 0
  fi

  local connect_output
  if ! connect_output="$(docker network connect "$WISP_LAB_NETWORK" "$WISP_BACKEND_CONTAINER" 2>&1)"; then
    if ! printf '%s' "$connect_output" | grep -Eiq 'already exists|is already connected'; then
      echo "Failed to connect $WISP_BACKEND_CONTAINER to $WISP_LAB_NETWORK. $connect_output" >&2
      return 1
    fi
  fi

  echo "Connected $WISP_BACKEND_CONTAINER to $WISP_LAB_NETWORK" >&2
  return 0
}

wisp_disconnect_backend_from_lab_network() {
  if ! wisp_backend_exists; then
    return 1
  fi

  if ! wisp_docker_object_exists network "$WISP_LAB_NETWORK"; then
    return 1
  fi

  if ! wisp_container_on_network "$WISP_BACKEND_CONTAINER" "$WISP_LAB_NETWORK"; then
    return 1
  fi

  local disconnect_output
  if ! disconnect_output="$(docker network disconnect "$WISP_LAB_NETWORK" "$WISP_BACKEND_CONTAINER" 2>&1)"; then
    if ! printf '%s' "$disconnect_output" | grep -Eiq 'is not connected|No such container|No such network'; then
      echo "Failed to disconnect $WISP_BACKEND_CONTAINER from $WISP_LAB_NETWORK. $disconnect_output" >&2
      return 1
    fi
  fi

  echo "Disconnected $WISP_BACKEND_CONTAINER from $WISP_LAB_NETWORK" >&2
  return 0
}

wisp_seed_target_prefix() {
  local target_mode="${1:-${WISP_SEED_TARGET_MODE:-auto}}"
  target_mode="$(printf '%s' "$target_mode" | tr '[:upper:]' '[:lower:]')"

  case "$target_mode" in
    docker)
      if ! wisp_connect_backend_to_lab_network; then
        echo "WISP_SEED_TARGET_MODE=docker requires the '$WISP_BACKEND_CONTAINER' container and '$WISP_LAB_NETWORK' network to be running." >&2
        return 1
      fi
      echo "Using WISP Docker management targets ${WISP_DOCKER_TARGET_PREFIX}21-${WISP_DOCKER_TARGET_PREFIX}42" >&2
      printf '%s' "$WISP_DOCKER_TARGET_PREFIX"
      ;;
    host)
      echo "Using WISP host loopback targets ${WISP_HOST_TARGET_PREFIX}21-${WISP_HOST_TARGET_PREFIX}42" >&2
      printf '%s' "$WISP_HOST_TARGET_PREFIX"
      ;;
    auto)
      if wisp_backend_running && wisp_connect_backend_to_lab_network; then
        echo "Using WISP Docker management targets ${WISP_DOCKER_TARGET_PREFIX}21-${WISP_DOCKER_TARGET_PREFIX}42" >&2
        printf '%s' "$WISP_DOCKER_TARGET_PREFIX"
      else
        echo "Using WISP host loopback targets ${WISP_HOST_TARGET_PREFIX}21-${WISP_HOST_TARGET_PREFIX}42" >&2
        printf '%s' "$WISP_HOST_TARGET_PREFIX"
      fi
      ;;
    *)
      echo "Invalid WISP seed target mode '$target_mode'. Use auto, docker, or host." >&2
      return 1
      ;;
  esac
}
