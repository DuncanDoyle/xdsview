# Scenario 03: Multiple Policies

HTTP Gateway with one HTTPRoute and three `TrafficPolicy`s targeting it: CORS, ext auth and global rate limit. The ext auth and rate limit `GatewayExtension`s point at Services with no pods; Envoy only needs their clusters to exist.

## What to observe

- One `merge.TrafficPolicy.gateway.kgateway.dev` metadata entry per policy field on the route
- The `ext_auth/kgateway-system/ext-authz` and `ratelimit/kgateway-system/ratelimit` filters, enabled per route

## K8S resources

- `k8s/gateway.yaml`, `k8s/httpbin.yaml`, `k8s/httproute-kgateway-system_service-httpbin-rg.yaml`: as in 02_7
- `k8s/api-example-com-httproute.yaml`: one rule to `httpbin`
- `k8s/cors-trafficpolicy.yaml`, `k8s/ext-auth-trafficpolicy.yaml`, `k8s/rate-limit-trafficpolicy.yaml`: the 02_6, 02_7 and 02_8 policies

## Envoy dump

`envoy/kgateway-<version>/config_dump.json`, with `server_info.json`.

## How to collect

```bash
hack/record-dumps.sh v2.5.0 03-multi-policy
```
