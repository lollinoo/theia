#!/usr/bin/env bash
set -euo pipefail
tools_dir=${1:?Usage: install-kubernetes-test-tools.sh directory}
mkdir -p "$tools_dir"
tools_dir=$(cd "$tools_dir" && pwd)
cd "$tools_dir"
curl -fsSL https://get.helm.sh/helm-v4.3.0-linux-amd64.tar.gz -o helm-v4.3.0-linux-amd64.tar.gz
curl -fsSL https://get.helm.sh/helm-v4.3.0-linux-amd64.tar.gz.sha256sum -o helm.sha256
sha256sum --check helm.sha256
tar -xzf helm-v4.3.0-linux-amd64.tar.gz linux-amd64/helm
cp linux-amd64/helm helm
curl -fsSL https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-amd64 -o kind-linux-amd64
curl -fsSL https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-amd64.sha256sum -o kind.sha256
sha256sum --check kind.sha256
mv kind-linux-amd64 kind
curl -fsSL https://dl.k8s.io/release/v1.37.0/bin/linux/amd64/kubectl -o kubectl
curl -fsSL https://dl.k8s.io/release/v1.37.0/bin/linux/amd64/kubectl.sha256 -o kubectl.sha256
printf '%s  kubectl\n' "$(cat kubectl.sha256)" | sha256sum --check
chmod +x kind kubectl helm
