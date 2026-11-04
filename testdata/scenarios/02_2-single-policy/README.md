# Scenario 02_2: Single Policy — RequestHeaderModifier (K8S HTTPRoute filter)

HTTPS Gateway with one HTTPRoute that contains a RequestHeaderModifier filter.

This is the on the first scenarios where we can observe how kgateway translates a K8S policy into an Envoy filter, and what metadata or naming conventions link the two.

## What to observe

- How does the HTTPRoute Filter appear in the Envoy config? As an HTTP filter, a per-route config, or both?
- Is there any metadata in the Envoy config that references the originating K8S resource?
- How does the filter chain differ from the no-policy baseline (scenario 01)?


## K8S resources

- `k8s/httpbin.yaml` — Single rule routing to one backend service
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api_developer-example-com-httproute.yaml` — Single rule routing to one backend service
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — Reference Grant granting access from the HTTPRoute in the kgateway-system namespace to Services in the httpbin namespace

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_2-single-policy
```
