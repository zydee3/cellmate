#!/usr/bin/env bash
# Build, load into k3s containerd, apply, restart. Run from anywhere.
set -euo pipefail
export PATH=/usr/local/go/bin:$PATH
export KUBECONFIG=${KUBECONFIG:-$HOME/.kube/config}
HOST=cellmate.10.0.0.209.sslip.io
cd "$(dirname "$0")/.."

go build -o /dev/null ./...
docker build -t cellmate:latest .
docker save cellmate:latest | sudo k3s ctr images import -
# imagePullPolicy Never: a pod with no image in k3s fails with ErrImageNeverPull.
id=$(docker image inspect -f '{{.Id}}' cellmate:latest)
sudo k3s ctr images ls name==docker.io/library/cellmate:latest | grep -q "$id" ||
  { echo "cellmate:latest $id not in k3s after import" >&2; exit 1; }

kubectl apply -f deploy/k8s.yaml
if ! kubectl -n cellmate get secret cellmate-tls >/dev/null 2>&1; then
  d=$(mktemp -d)
  openssl req -x509 -newkey rsa:2048 -nodes -days 825 -subj "/CN=$HOST" \
    -addext "subjectAltName=DNS:$HOST,IP:10.0.0.209" \
    -keyout "$d/tls.key" -out "$d/tls.crt"
  kubectl -n cellmate create secret tls cellmate-tls --cert="$d/tls.crt" --key="$d/tls.key"
  rm -rf "$d"
fi
kubectl -n cellmate rollout restart deploy/cellmate
# Recreate waits for the old pod to exit, which takes minutes under high load.
kubectl -n cellmate rollout status deploy/cellmate --timeout=600s
echo "https://$HOST:4443/"
