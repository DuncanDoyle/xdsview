# Scenario 01: Simple

HTTP Gateway with a single listener, one HTTPRoute, one backend, a PathPrefix matcher and a URLRewrite HTTPRouteFilter.

This is one the simplest scenarios that uses a matcher.

## What to observe

- Which HTTP filters does kgateway inject by default (even with no policies)?
- What does the cluster name look like for the backend service?
- How is the HTTPRoute represented in the route config (VirtualHost name, domains, route match)?
- How is matcher represented in the route config?

## K8S resources

- `k8s/httpbin.yaml` — Single rule routing to one backend service
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api-example-com-httproute.yaml` — Single rule with a single mather and a URLRewrite HTTPRouteFilter routing to one backend service
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — Reference Grant granting access from the HTTPRoute in the kgateway-system namespace to Services in the httpbin namespace

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_1-matchers
```
