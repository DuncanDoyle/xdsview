# Scenario 02_7: Single Policy — ExtAuth (TrafficPolicy)

HTTP Gateway with one HTTPRoute and a `TrafficPolicy` that enables external authorization through an `ExtAuth` `GatewayExtension`.

The extension points at a Service with no pods. Envoy's `ext_authz` filter config only needs the cluster to exist, so no auth server runs.

## What to observe

- How does the `envoy.filters.http.ext_authz` filter appear in the filter chain?
- What cluster is created for the ext auth service?
- Is there per-route config that enables/disables ext authz?
- How does the GatewayExtension reference appear in the Envoy config (filter name, cluster)?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Simple HTTPRoute with no filters
- `k8s/ext-auth-trafficpolicy.yaml` — `ext-authz` Service (no pods), `ExtAuth` `GatewayExtension`, and the `TrafficPolicy` that uses it
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

HTTP Gateway with one HTTPRoute and a `TrafficPolicy` that enables external authorization through an `ExtAuth` `GatewayExtension`. The extension points at a Service with no pods: Envoy's `ext_authz` filter config only needs the cluster to exist, so no auth server runs.

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_7-single-policy
```
