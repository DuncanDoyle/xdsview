# Scenario 02_6: Single Policy — CORS (TrafficPolicy)

HTTP Gateway with one HTTPRoute and a `TrafficPolicy` with a `cors` section.

Contrast with an HTTPRoute-level CORS filter to see where CORS ends up in the Envoy config when it comes from a policy.

## What to observe

- Does CORS appear as an HTTP filter in the filter chain, as per-route typed config, or both?
- What is the Envoy filter name for CORS (`envoy.filters.http.cors`)?
- How are the CORS allow-origins, allow-methods, etc. encoded in the Envoy config?
- Is the CORS filter placed before or after other filters in the chain?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Simple HTTPRoute with no filters (policy attached separately)
- `k8s/cors-trafficpolicy.yaml` — `TrafficPolicy` with `cors`
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

HTTP Gateway with one HTTPRoute and a `TrafficPolicy` with a `cors` section.

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_6-single-policy
```
