#!/usr/bin/env bash
set -euo pipefail
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_dir"
for command in kind kubectl helm docker go python3 curl; do
  command -v "$command" >/dev/null || { echo "Missing test prerequisite: $command" >&2; exit 1; }
done
test_root=$(mktemp -d /tmp/theia-kubernetes-XXXXXX)
cluster_name="theia-test-$RANDOM-$RANDOM"
export KUBECONFIG="$test_root/kubeconfig"
forward_pid=''
cleanup() {
  if [[ -n "$forward_pid" ]]; then kill "$forward_pid" 2>/dev/null || true; fi
  kind delete cluster --name "$cluster_name" >/dev/null 2>&1 || true
  python3 - "$test_root" <<'PY'
import pathlib,shutil,sys
root=pathlib.Path(sys.argv[1]); assert root.parent==pathlib.Path('/tmp') and root.name.startswith('theia-kubernetes-')
shutil.rmtree(root,ignore_errors=True)
PY
}
trap cleanup EXIT
cat > "$test_root/kind.yaml" <<'YAML'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
YAML
kind create cluster --name "$cluster_name" --kubeconfig "$KUBECONFIG" --config "$test_root/kind.yaml" --image kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5 --wait 120s
context="kind-$cluster_name"
# Let both nodes host the application to exercise affinity rather than a single
# eligible worker masked by the control-plane taint.
kubectl --context "$context" taint node "$cluster_name-control-plane" node-role.kubernetes.io/control-plane-
docker build --target production -t theia-managed-backend:local .
docker build --target production -f Dockerfile.frontend -t theia-managed-frontend:local frontend
docker pull postgres:18-bookworm
docker save theia-managed-backend:local theia-managed-frontend:local postgres:18-bookworm -o "$test_root/images.tar"
# Explicit platform avoids importing absent ARM manifests from Docker's image store.
while IFS= read -r test_node; do
  docker exec --privileged -i "$test_node" ctr -n k8s.io images import --platform linux/amd64 --snapshotter overlayfs - < "$test_root/images.tar"
done < <(kind get nodes --name "$cluster_name")
python3 - "$test_root/images.tar" <<'PY'
import pathlib,sys
pathlib.Path(sys.argv[1]).unlink()
PY
go build -o "$test_root/theia-admin" ./cmd/theia-admin
image_args=(-backend-image theia-managed-backend:local -frontend-image theia-managed-frontend:local)
"$test_root/theia-admin" install -platform kubernetes -dir "$test_root/source" -kube-context "$context" -namespace theia-source -name theia -hostname theia.test -tls-secret theia-tls -release v0.0.0-test "${image_args[@]}" > "$test_root/install.log" 2>&1
kubectl --context "$context" -n theia-source exec deployment/theia-backend -c backend -- theia instance status > "$test_root/expected.json"
[[ -z $(kubectl --context "$context" -n theia-source get secret theia-bootstrap --ignore-not-found -o name) ]]
forward() {
  if [[ -n "$forward_pid" ]]; then kill "$forward_pid" 2>/dev/null || true; wait "$forward_pid" 2>/dev/null || true; fi
  test_port=$(python3 - <<'PY'
import socket
s=socket.socket();s.bind(('127.0.0.1',0));print(s.getsockname()[1])
PY
)
  kubectl --context "$context" -n "$1" port-forward service/theia-frontend "$test_port:80" > "$test_root/forward.log" 2>&1 &
  forward_pid=$!
  for attempt in $(seq 1 60); do
    if curl -fsS -o /dev/null "http://127.0.0.1:$test_port/readyz" 2>/dev/null; then return; fi
    sleep 1
  done
  echo 'Port-forward did not become ready' >&2; return 1
}
forward theia-source
activation_token=$(python3 - "$test_root/install.log" <<'PY'
import pathlib,re,sys
print(re.search(r'token=([A-Za-z0-9_-]+)',pathlib.Path(sys.argv[1]).read_text()).group(1))
PY
)
THEIA_MANAGED_URL="http://127.0.0.1:$test_port" THEIA_ACTIVATION_URL="http://127.0.0.1:$test_port/activate#token=$activation_token" THEIA_OPERATOR_RECOVERY_FILE="$test_root/recovery.txt" frontend/node_modules/.bin/playwright test --config frontend/playwright.managed.config.ts
"$test_root/theia-admin" backup -dir "$test_root/source" -output "$test_root/instance.age" > "$test_root/backup.log" 2>&1
kubectl --context "$context" -n theia-source rollout restart deployment/theia-backend
kubectl --context "$context" -n theia-source rollout status deployment/theia-backend --timeout=180s
kubectl --context "$context" -n theia-source exec deployment/theia-backend -c backend -- theia instance status > "$test_root/restarted.json"
python3 - "$test_root" <<'PY'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1]);expected=json.loads((root/'expected.json').read_text());actual=json.loads((root/'restarted.json').read_text())
assert all(actual[key]==expected[key] for key in ['instance_id','active_key_id','retained_key_ids'])
PY
[[ $(kubectl --context "$context" -n theia-source exec deployment/theia-backend -c backend -- stat -c %a /persist/control/state.json) == 600 ]]
kubectl --context "$context" -n theia-source get pods -l theia-instance=theia -o json > "$test_root/consumers.json"
python3 - "$test_root/consumers.json" <<'PY'
import json,pathlib,sys
pods=json.loads(pathlib.Path(sys.argv[1]).read_text())['items']
assert len(pods)==2 and len({pod['spec']['nodeName'] for pod in pods})==1
PY
# A second writer must fail without changing the persistent identity.
if kubectl --context "$context" -n theia-source exec deployment/theia-backend -c backend -- theia maintenance migrate > "$test_root/concurrent.log" 2>&1; then echo 'Concurrent maintenance writer accepted' >&2; exit 1; fi
"$test_root/theia-admin" upgrade -dir "$test_root/source" -release v0.0.1-test "${image_args[@]}" > "$test_root/upgrade.log" 2>&1
"$test_root/theia-admin" migrate -dir "$test_root/source" -rotate-credentials > "$test_root/credentials.log" 2>&1
"$test_root/theia-admin" rotate-operational -dir "$test_root/source" > "$test_root/operational.log" 2>&1
"$test_root/theia-admin" rotate-recovery -dir "$test_root/source" -recovery-file "$test_root/recovery.txt" -output "$test_root/history.txt" > "$test_root/recovery.log" 2>&1
"$test_root/theia-admin" restore -platform kubernetes -dir "$test_root/replacement" -kube-context "$context" -namespace theia-replacement -name theia -hostname theia.test -tls-secret theia-tls -release v0.0.1-test -archive "$test_root/instance.age" -recovery-file "$test_root/history.txt" "${image_args[@]}" > "$test_root/restore.log" 2>&1
forward theia-replacement
THEIA_MANAGED_URL="http://127.0.0.1:$test_port" frontend/node_modules/.bin/playwright test --config frontend/playwright.managed.config.ts
kubectl --context "$context" -n theia-replacement exec deployment/theia-backend -c backend -- theia instance status > "$test_root/restored.json"
python3 - "$test_root" <<'PY'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1]);expected=json.loads((root/'expected.json').read_text());actual=json.loads((root/'restored.json').read_text())
assert all(actual[key]==expected[key] for key in ['instance_id','active_key_id','retained_key_ids'])
(root/'corrupt.age').write_bytes((root/'instance.age').read_bytes()[:64])
PY
if "$test_root/theia-admin" restore -dir "$test_root/replacement" -archive "$test_root/corrupt.age" -recovery-file "$test_root/history.txt" > "$test_root/corrupt.log" 2>&1; then echo 'Truncated archive accepted' >&2; exit 1; fi
curl -fsS -o /dev/null "http://127.0.0.1:$test_port/readyz"
[[ -z $(kubectl --context "$context" -n theia-replacement get secrets -o name | rg 'theia-recovery-' || true) ]]
# Losing state after removing the seed must block startup, not generate new keys.
kubectl --context "$context" -n theia-source exec deployment/theia-backend -c backend -- mv /persist/control/state.json /persist/control/lost-state.json
kubectl --context "$context" -n theia-source rollout restart deployment/theia-backend
for attempt in $(seq 1 60); do
  if kubectl --context "$context" -n theia-source logs deployment/theia-backend -c bootstrap 2>/dev/null | rg -q 'refusing to generate replacement keys'; then break; fi
  sleep 1
done
kubectl --context "$context" -n theia-source logs deployment/theia-backend -c bootstrap 2>/dev/null | rg -q 'refusing to generate replacement keys'
echo 'Kubernetes activation, restart, upgrades, rotations, replacement restore and failure checks passed.'
