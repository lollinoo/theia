# shellcheck shell=bash
# Shared CI helper functions (sourced, not executed).

# Wait up to 60x2s for the PostgreSQL TCP endpoint in a postgres:// DSN to accept connections.
wait_for_postgres() {
  local dsn="${1-}"
  local authority host port i

  if [[ -z "$dsn" || ( "$dsn" != postgres://* && "$dsn" != postgresql://* ) ]]; then
    echo "wait_for_postgres: invalid or missing PostgreSQL DSN" >&2
    return 1
  fi

  authority="${dsn#*://}"
  authority="${authority%%/*}"
  authority="${authority%%\?*}"
  authority="${authority##*@}"

  if [[ "$authority" == *:* ]]; then
    host="${authority%%:*}"
    port="${authority##*:}"
  else
    host="$authority"
    port="5432"
  fi

  if [[ -z "$host" || -z "$port" || ! "$port" =~ ^[0-9]+$ ]]; then
    echo "wait_for_postgres: could not parse host and port from DSN" >&2
    return 1
  fi

  for ((i = 1; i <= 60; i++)); do
    if (exec 3<>"/dev/tcp/${host}/${port}") 2>/dev/null; then
      return 0
    fi
    if (( i < 60 )); then
      sleep 2
    fi
  done

  echo "wait_for_postgres: timed out waiting for PostgreSQL at ${host}:${port}" >&2
  return 1
}

# Sanitize a raw ref or release name into a valid OCI/Docker image tag (<= 128 chars).
sanitize_tag() {
  local raw="${1-}"
  local tag

  tag="$(printf '%s\n' "$raw" | LC_ALL=C tr '[:upper:]' '[:lower:]' | LC_ALL=C sed -E 's|[^a-z0-9_.-]+|-|g; s|^-+||; s|-+$||')"

  if [[ -z "$tag" ]]; then
    echo "sanitize_tag: tag is empty after sanitization" >&2
    return 1
  fi

  if (( ${#tag} > 128 )); then
    echo "sanitize_tag: tag exceeds 128 characters (${#tag})" >&2
    return 1
  fi

  printf '%s\n' "$tag"
}
