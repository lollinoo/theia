#!/usr/bin/env bash
set -euo pipefail

# Socket-backed CI containers need the daemon's network and identical bind paths.
# The existing trusted-volume permission is sufficient; no new CI network trust
# or persistent product secrets are required. Serialize image tags on that daemon.
repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
ci_shared_root=/tmp/theia-managed-ci
mkdir -p "$ci_shared_root"
chmod 700 "$ci_shared_root"
exec 9> "$ci_shared_root/deployment.lock"
flock 9
# Wait for any cancelled pipeline's runner to finish its own cleanup before clearing --rm lag.
docker wait theia-managed-ci-runner >/dev/null 2>&1 || true
docker rm -f theia-managed-ci-runner >/dev/null 2>&1 || true
ci_work=$(mktemp -d "$ci_shared_root/run-XXXXXX")
trap 'rm -rf "$ci_work"' EXIT
mkdir -p "$ci_work/repo" "$ci_work/tmp"
if [[ $# -eq 0 ]]; then
  set -- deployment-test maintenance-test postgres-upgrade-test kubernetes-test
fi

# Only tracked files enter the runner; ignored local secrets never travel with it.
git -c safe.directory="$repo_dir" -C "$repo_dir" ls-files -z \
  | tar -C "$repo_dir" --null -T - -cf - \
  | tar -C "$ci_work/repo" -xf -
# Chromium system packages live in an image layer that rebuilds only when Playwright changes.
playwright_version=$(awk '/"node_modules\/playwright-core": \{/{f=1} f&&/"version"/{gsub(/[",]/,"",$2); print $2; exit}' "$repo_dir/frontend/package-lock.json")
docker build -f "$repo_dir/Dockerfile.deployment-test" --build-arg PLAYWRIGHT_VERSION="$playwright_version" \
  -t theia-managed-ci-runner:local "$repo_dir"
# Host-side caches shared with the other CI steps; all are safe for concurrent use.
cache=/var/cache/theia-ci
docker run --rm --name theia-managed-ci-runner --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$ci_work:$ci_work" -w "$ci_work/repo" \
  -v "$cache:$cache" \
  -e npm_config_cache="$cache/npm" -e GOMODCACHE="$cache/gomod" -e GOCACHE="$cache/gobuild" \
  -e PLAYWRIGHT_BROWSERS_PATH="$cache/playwright" \
  -e TMPDIR="$ci_work/tmp" -e CI=true \
  theia-managed-ci-runner:local bash -ec '
    npm --prefix frontend ci
    frontend/node_modules/.bin/playwright install chromium
    # Tool versions are pinned in the installer, so its hash keys the cache.
    tools="/var/cache/theia-ci/k8s-tools/$(sha256sum scripts/install-kubernetes-test-tools.sh | cut -c1-16)"
    [ -x "$tools/kubectl" ] || bash scripts/install-kubernetes-test-tools.sh "$tools"
    export PATH="$tools:$PATH"
    make "$@"
  ' managed-ci "$@"
