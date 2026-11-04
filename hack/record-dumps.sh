#!/usr/bin/env bash
# record-dumps.sh <kgateway-version> [scenario ...]
#
# Records Envoy config dumps for the OSS test corpus on a throwaway kind cluster.
# For each scenario in testdata/scenarios that has an HTTPRoute manifest (or the
# named ones), it writes envoy/kgateway-<version>/config_dump.json and the
# proxy's server_info.json. The cluster gets its own kubeconfig file, so
# ~/.kube/config is never read or changed, and it is deleted on exit.
set -euo pipefail

version=${1:?usage: $0 <kgateway-version> [scenario ...]}
shift
repo=$(cd "$(dirname "$0")/.." && pwd)
# A per-run name, so concurrent runs and existing clusters are never touched.
cluster=krp-record-$$
context=kind-$cluster
kubeconfig=$(mktemp)
pf_pid=
created=

cleanup() {
  [ -n "$pf_pid" ] && kill "$pf_pid" 2>/dev/null || true
  # Delete only a cluster this run created.
  [ -n "$created" ] && kind delete cluster --name "$cluster" --kubeconfig "$kubeconfig" >/dev/null 2>&1 || true
  rm -f "$kubeconfig"
}
trap cleanup EXIT

k() { kubectl --kubeconfig "$kubeconfig" --context "$context" "$@"; }
h() { helm --kubeconfig "$kubeconfig" --kube-context "$context" "$@"; }

# Install the Gateway API minor this kgateway version is built against.
gwapi=$(curl -fsSL "https://raw.githubusercontent.com/kgateway-dev/kgateway/$version/go.mod" |
  awk '$1 == "sigs.k8s.io/gateway-api" {print $2}')
echo "kgateway $version, Gateway API $gwapi"

if kind get clusters 2>/dev/null | grep -qx "$cluster"; then
  echo "kind cluster $cluster already exists; not touching it" >&2
  exit 1
fi
kind create cluster --name "$cluster" --kubeconfig "$kubeconfig" --wait 120s
created=1
k apply --server-side -f "https://github.com/kubernetes-sigs/gateway-api/releases/download/$gwapi/experimental-install.yaml"
h upgrade -i kgateway-crds oci://cr.kgateway.dev/kgateway-dev/charts/kgateway-crds \
  --version "$version" -n kgateway-system --create-namespace --wait
h upgrade -i kgateway oci://cr.kgateway.dev/kgateway-dev/charts/kgateway \
  --version "$version" -n kgateway-system --wait --timeout 5m

if [ $# -gt 0 ]; then
  scenarios=("$@")
else
  scenarios=()
  for d in "$repo"/testdata/scenarios/*/; do
    grep -qsl '^kind: HTTPRoute' "$d"/k8s/*.yaml && scenarios+=("$(basename "$d")")
  done
fi

# route_markers prints the "httproute-<name>-<namespace>-" marker of every HTTPRoute
# in the scenario; the dump is complete when Envoy has a route for each.
route_markers() {
  k get -f "$1/k8s" -o json 2>/dev/null |
    jq -r '.items[]? // . | select(.kind == "HTTPRoute") | "httproute-\(.metadata.name)-\(.metadata.namespace)-"'
}

# has_all reports whether the dump in $1 contains every marker line in $2.
has_all() {
  local m
  while read -r m; do
    [ -z "$m" ] || grep -qF -- "$m" <<<"$1" || return 1
  done <<<"$2"
}

# fetch_settled polls /config_dump until every marker is present and two reads
# in a row are equal, then writes the dump to $2.
fetch_settled() {
  local port=$1 out=$2 dir=$3 prev= cur= markers
  markers=$(route_markers "$dir")
  for _ in $(seq 1 60); do
    cur=$(curl -fsS "localhost:$port/config_dump" | jq . 2>/dev/null) || cur=
    if [ -n "$cur" ] && [ "$cur" = "$prev" ] && has_all "$cur" "$markers"; then
      printf '%s\n' "$cur" >"$out"
      return 0
    fi
    prev=$cur
    sleep 2
  done
  echo "config_dump did not settle for $dir: markers present: $(has_all "$cur" "$markers" && echo yes || echo no), stable: $([ "$cur" = "$prev" ] && echo yes || echo no)" >&2
  printf '%s\n' "$cur" >"$out.unsettled"
  return 1
}

for s in "${scenarios[@]}"; do
  dir=$repo/testdata/scenarios/$s
  out=$dir/envoy/kgateway-$version
  echo "== $s"
  "$repo/hack/scenario.sh" up "$dir" "$kubeconfig" "$context"

  port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1])')
  # kubectl directly, not through k(): $! must be kubectl's PID so kill stops it.
  kubectl --kubeconfig "$kubeconfig" --context "$context" port-forward -n kgateway-system deployment/gw "$port:19000" >/dev/null 2>&1 &
  pf_pid=$!
  for _ in $(seq 1 30); do curl -fsS "localhost:$port/ready" >/dev/null 2>&1 && break; sleep 1; done

  mkdir -p "$out"
  fetch_settled "$port" "$out/config_dump.json" "$dir"
  curl -fsS "localhost:$port/server_info" | jq '{version, state, command_line_options: {concurrency: .command_line_options.concurrency}}' >"$out/server_info.json"

  kill "$pf_pid"
  wait "$pf_pid" 2>/dev/null || true
  pf_pid=
  "$repo/hack/scenario.sh" down "$dir" "$kubeconfig" "$context"
done
echo "recorded ${#scenarios[@]} scenarios for kgateway $version"
