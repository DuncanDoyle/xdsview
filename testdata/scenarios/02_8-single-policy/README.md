# Scenario 02_8: Single Policy — RateLimit (TrafficPolicy)

HTTP Gateway with one HTTPRoute and a `TrafficPolicy` with a global rate limit through a `RateLimit` `GatewayExtension`, using the `X-Custom` header as descriptor. The extension points at a Service with no pods; no rate limit server runs.

## What to observe

- How does the `envoy.filters.http.ratelimit` filter appear in the filter chain?
- What cluster is created for the rate limit service?
- How are the rate limit descriptors/actions encoded in the per-route config?
- Is there a separate rate limit service cluster in the Envoy cluster config?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Simple HTTPRoute with no filters
- `k8s/rate-limit-trafficpolicy.yaml` — `ratelimit` Service (no pods), `RateLimit` `GatewayExtension`, and the `TrafficPolicy` that uses it
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

HTTP Gateway with one HTTPRoute and a `TrafficPolicy` with a global rate limit through a `RateLimit` `GatewayExtension` (descriptor from the `X-Custom` header). The extension points at a Service with no pods; no rate limit server runs.

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_8-single-policy
```
