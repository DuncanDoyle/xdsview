#!/usr/bin/env bash
# scenario.sh up|down <scenario-dir> <kubeconfig> <context>
#
# Applies or removes one test scenario from testdata/scenarios. "up" runs the
# scenario's k8s/hook-*.sh scripts, applies every manifest in k8s/, and waits
# until the gateway is programmed and its routes and policies are Accepted.
# "down" deletes everything "up" created and waits for the proxy pod to go, so
# the next scenario starts from a fresh Envoy.
set -euo pipefail

action=$1 dir=$2 kubeconfig=$3 context=$4
k() { kubectl --kubeconfig "$kubeconfig" --context "$context" "$@"; }

# accepted <kind> waits until every <kind> in the scenario reports Accepted=True.
# HTTPRoutes report per parent, TrafficPolicies per ancestor.
accepted() {
  local kind=$1 path=$2
  for obj in $(k get -f "$dir/k8s" -o name 2>/dev/null | grep -i "^$kind" || true); do
    k wait -n kgateway-system --for=jsonpath="{$path[0].conditions[?(@.type==\"Accepted\")].status}"=True "$obj" --timeout=120s
  done
}

case $action in
up)
  k create namespace httpbin --dry-run=client -o yaml | k apply -f -
  for hook in "$dir"/k8s/hook-*.sh; do [ -e "$hook" ] && bash "$hook" up "$kubeconfig" "$context"; done
  k apply -f "$dir/k8s"
  # rollout status, not a label wait: right after apply there may be no pod to match yet.
  k rollout status -n httpbin deployment/httpbin --timeout=180s
  k wait -n kgateway-system --for=condition=Programmed gateway/gw --timeout=180s
  # The first scenario on a fresh cluster also pulls the proxy image.
  if ! k rollout status -n kgateway-system deployment/gw --timeout=300s; then
    k get pods -n kgateway-system -o wide >&2
    k describe pods -n kgateway-system -l gateway.networking.k8s.io/gateway-name=gw >&2
    exit 1
  fi
  accepted httproute .status.parents
  accepted trafficpolicy .status.ancestors
  ;;
down)
  k delete -f "$dir/k8s" --ignore-not-found --wait --timeout=180s
  for hook in "$dir"/k8s/hook-*.sh; do [ -e "$hook" ] && bash "$hook" down "$kubeconfig" "$context"; done
  k delete namespace httpbin --ignore-not-found --wait --timeout=180s
  k wait -n kgateway-system --for=delete pod -l gateway.networking.k8s.io/gateway-name=gw --timeout=180s 2>/dev/null || true
  ;;
*)
  echo "usage: $0 up|down <scenario-dir> <kubeconfig> <context>" >&2
  exit 2
  ;;
esac
