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
docker build -f "$repo_dir/Dockerfile.deployment-test" -t theia-managed-ci-runner:local "$repo_dir"
docker run --rm --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v "$ci_work:$ci_work" -w "$ci_work/repo" \
  -e TMPDIR="$ci_work/tmp" -e CI=true \
  theia-managed-ci-runner:local bash -ec '
    npm --prefix frontend ci
    npm --prefix frontend run e2e:install
    bash scripts/install-kubernetes-test-tools.sh "$TMPDIR/tools"
    export PATH="$TMPDIR/tools:$PATH"
    make "$@"
  ' managed-ci "$@"
