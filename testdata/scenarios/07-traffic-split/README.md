# Scenario 07: Traffic Split

One HTTPRoute rule with two weighted backendRefs: `httpbin` (80) and `httpbin-v2` (20).

## What to observe

- The route action uses `weighted_clusters` instead of `cluster` (#13)

## K8S resources

- `k8s/gateway.yaml`, `k8s/httpbin.yaml`, `k8s/httproute-kgateway-system_service-httpbin-rg.yaml`: as in 02_7
- `k8s/httpbin-v2-svc.yaml`: a second Service for the httpbin pods
- `k8s/api-example-com-httproute.yaml`: the weighted rule

## Envoy dump

`envoy/kgateway-<version>/config_dump.json`, with `server_info.json`.

## How to collect

```bash
hack/record-dumps.sh v2.5.0 07-traffic-split
```
