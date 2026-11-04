# Scenario 02_3: Single Policy — ResponseHeaderModifier (K8S HTTPRoute filter)

HTTP Gateway with one HTTPRoute that contains a `ResponseHeaderModifier` filter.

Companion to 02_2 (RequestHeaderModifier), but on the response path. Useful for comparing where request vs response header manipulation lands in the Envoy config.

## What to observe

- Does response header modification appear in the same place as request header modification (per-route config), or somewhere else?
- Is there any Envoy filter involved, or is it purely route-level config?
- How does the filter chain compare to the no-policy baseline (scenario 01)?

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — HTTPRoute with ResponseHeaderModifier filter
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_3-single-policy
```
