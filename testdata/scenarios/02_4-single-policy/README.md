# Scenario 02_4: Single Policy — RequestMirror (K8S HTTPRoute filter)

HTTP Gateway with one HTTPRoute that uses a `RequestMirror` filter to shadow traffic to a second backend.

Interesting from a filter chain perspective because mirroring is a cluster-level concern in Envoy, not a header manipulation — useful for seeing how kgateway translates a K8S filter into Envoy route config.

## What to observe

- How does request mirroring appear in the Envoy route config? Is it a `request_mirror_policies` entry on the route?
- Does a separate cluster get created for the mirror backend?
- Is there any HTTP filter involved, or is it purely route-level config?

## K8S resources

- `k8s/httpbin.yaml` — Primary HTTPBin backend (`httpbin` service)
- `k8s/httpbin-mirror.yaml` — Mirror HTTPBin backend (`httpbin-mirror` service)
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — HTTPRoute with RequestMirror filter pointing at httpbin-mirror
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace (covers both services)

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_4-single-policy
```
