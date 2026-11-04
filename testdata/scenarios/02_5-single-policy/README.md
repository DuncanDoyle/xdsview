# Scenario 02_5: Single Policy — URLRewrite (K8S HTTPRoute filter)

HTTP Gateway with one HTTPRoute that uses a `URLRewrite` filter to rewrite the request path before forwarding to the backend.

## What to observe

- How does path rewriting appear in the Envoy route config? As a `prefix_rewrite` or `regex_rewrite` on the route?
- Is there any HTTP filter involved, or is it purely route-level config?
- How does the route match interact with the rewrite (prefix match required for prefix rewrite)?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — HTTPRoute with URLRewrite filter (path prefix rewrite)
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_5-single-policy
```
