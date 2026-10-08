#!/usr/bin/env bash
set -euo pipefail

# This harness owns two temporary Compose projects. It never uses the standard
# project name, host database, production environment file or deployed volumes.
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
test_root=$(mktemp -d)
cleanup() {
  for instance_dir in "$test_root/source" "$test_root/replacement"; do
    if [[ -f "$instance_dir/compose.yaml" ]]; then
      docker compose -f "$instance_dir/compose.yaml" down --remove-orphans --volumes >/dev/null 2>&1 || true
    fi
  done
  # PostgreSQL and Caddy own files inside this test-only bind mount.
  docker run --rm -v "$test_root:/test" --entrypoint sh theia-managed-backend:local -c 'rm -rf /test/*' >/dev/null 2>&1 || true
  rm -rf "$test_root"
}
trap cleanup EXIT
cd "$repo_dir"

docker build --target production -t theia-managed-backend:local .
docker build --target production -f Dockerfile.frontend -t theia-managed-frontend:local frontend
go build -o "$test_root/theia-admin" ./cmd/theia-admin
read -r source_http source_https replacement_http replacement_https < <(python3 - <<'PY'
import socket
sockets = [socket.socket() for _ in range(4)]
for sock in sockets:
    sock.bind(('127.0.0.1', 0))
print(*(sock.getsockname()[1] for sock in sockets))
PY
)
image_args=(-backend-image theia-managed-backend:local -frontend-image theia-managed-frontend:local -offline)
mkdir -p "$test_root/operator"

"$test_root/theia-admin" install -dir "$test_root/source" -release v0.0.0-test \
  -hostname localhost -bind-address 127.0.0.1 -http-port "$source_http" -https-port "$source_https" \
  "${image_args[@]}" > "$test_root/install.log" 2>&1
"$test_root/theia-admin" ca -dir "$test_root/source" -output "$test_root/operator/ca.crt"
THEIA_MANAGED_URL="https://localhost:$source_https" \
THEIA_ACTIVATION_URL=$(sed -n '/activate#token=/p' "$test_root/install.log") \
THEIA_OPERATOR_RECOVERY_FILE="$test_root/operator/recovery.txt" \
  frontend/node_modules/.bin/playwright test --config frontend/playwright.managed.config.ts

"$test_root/theia-admin" backup -dir "$test_root/source" -output "$test_root/operator/instance.age" > "$test_root/backup.log" 2>&1
python3 - "$test_root" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
backup = next(json.loads(line) for line in (root/'backup.log').read_text().splitlines() if line.startswith('{"id":'))
assert backup['status'] == 'success'
assert (root/'operator/instance.age').is_file()
state = json.loads((root/'source/control/state.json').read_text())
(root/'expected.json').write_text(json.dumps({key: state[key] for key in ['instance_id', 'active_key_id', 'recovery_recipient']}))
PY
"$test_root/theia-admin" up -dir "$test_root/source" > "$test_root/restart.log" 2>&1
python3 - "$test_root" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
expected = json.loads((root/'expected.json').read_text())
state = json.loads((root/'source/control/state.json').read_text())
assert all(state[key] == value for key, value in expected.items()), 'restart replaced instance identity or keys'
PY
"$test_root/theia-admin" upgrade -dir "$test_root/source" -release v0.0.1-test \
  "${image_args[@]}" > "$test_root/upgrade.log" 2>&1
"$test_root/theia-admin" migrate -dir "$test_root/source" -rotate-credentials > "$test_root/rotation.log" 2>&1
python3 - "$test_root" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
expected = json.loads((root/'expected.json').read_text())
state = json.loads((root/'source/control/state.json').read_text())
assert state['active_key_id'] != expected['active_key_id'], 'rotation did not replace the active key'
assert expected['active_key_id'] in state['credential_keys'], 'historical keys were discarded'
PY

# A single restore command provisions the replacement database and uses the saved
# original recovery file. No manual credential-key or PostgreSQL password transfer.
"$test_root/theia-admin" restore -dir "$test_root/replacement" -release v0.0.1-test \
  -hostname localhost -bind-address 127.0.0.1 -http-port "$replacement_http" -https-port "$replacement_https" \
  -archive "$test_root/operator/instance.age" -recovery-file "$test_root/operator/recovery.txt" \
  "${image_args[@]}" > "$test_root/restore.log" 2>&1
"$test_root/theia-admin" ca -dir "$test_root/replacement" -output "$test_root/operator/restored-ca.crt"
cmp "$test_root/operator/ca.crt" "$test_root/operator/restored-ca.crt"
python3 - "$test_root" <<'PY'
import json, pathlib, sys
root = pathlib.Path(sys.argv[1])
expected = json.loads((root/'expected.json').read_text())
state = json.loads((root/'replacement/control/state.json').read_text())
assert all(state[key] == value for key, value in expected.items()), 'replacement restore lost original identity or keys'
assert 'AGE-SECRET-KEY-' not in (root/'replacement/control/state.json').read_text(), 'private recovery identity was persisted'
PY
THEIA_MANAGED_URL="https://localhost:$replacement_https" \
  frontend/node_modules/.bin/playwright test --config frontend/playwright.managed.config.ts

head -c 100 "$test_root/operator/instance.age" > "$test_root/operator/truncated.age"
if "$test_root/theia-admin" restore -dir "$test_root/replacement" \
  -archive "$test_root/operator/truncated.age" -recovery-file "$test_root/operator/recovery.txt" \
  > "$test_root/rejected-restore.log" 2>&1; then
  echo 'Truncated archive was accepted' >&2
  exit 1
fi
curl --fail --silent --cacert "$test_root/operator/ca.crt" "https://localhost:$replacement_https/readyz" >/dev/null
echo 'Verified first activation, persistent restart, upgrade, key rotation, replacement restore, CA continuity and preflight rejection.'
