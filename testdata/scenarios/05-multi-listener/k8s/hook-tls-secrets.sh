#!/usr/bin/env bash
# hook-tls-secrets.sh up|down <kubeconfig> <context>
# Creates (up) or deletes (down) self-signed TLS secrets for the HTTPS listeners.
set -euo pipefail
action=$1 kubeconfig=$2 context=$3
k() { kubectl --kubeconfig "$kubeconfig" --context "$context" -n kgateway-system "$@"; }

for host in api.example.com; do
  secret=${host//./-}
  if [ "$action" = down ]; then
    k delete secret "$secret" --ignore-not-found
    continue
  fi
  tmp=$(mktemp -d)
  openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout "$tmp/tls.key" -out "$tmp/tls.crt" -subj "/CN=$host" 2>/dev/null
  k create secret tls "$secret" --key "$tmp/tls.key" --cert "$tmp/tls.crt" --dry-run=client -o yaml | k apply -f -
  rm -r "$tmp"
done
