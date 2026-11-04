# Scenario 04: Multiple Rules

One HTTPRoute with three rules and two Services:

| Rule | Matches | Backend |
|---|---|---|
| 0 | PathPrefix `/api` | `httpbin` |
| 1 | PathPrefix `/web`; header `x-version: v2` (two matches) | `httpbin-v2` |
| 2 | none (`/`) | `httpbin` |

## What to observe

- Envoy route names carry `-<ruleIdx>-<matchIdx>-` after the namespace. The `-route-<N>-` position is the order after precedence sorting, not the rule index.
- Rule 1 produces two Envoy routes, one per match.

## K8S resources

- `k8s/gateway.yaml`, `k8s/httpbin.yaml`, `k8s/httproute-kgateway-system_service-httpbin-rg.yaml`: as in 02_7
- `k8s/httpbin-v2-svc.yaml`: a second Service for the httpbin pods
- `k8s/api-example-com-httproute.yaml`: the three rules

## Envoy dump

- `envoy/kgateway-<version>/`: default install
- `envoy/kgateway-<version>-route-source/`: recorded with `enableRouteSourceMetadata: true` (`hack/route-source-values.yaml`), so every route carries `dev.kgateway.route_source` metadata

## How to collect

```bash
hack/record-dumps.sh v2.5.0 04-multi-rule
hack/record-dumps.sh --values hack/route-source-values.yaml --suffix -route-source v2.5.0 04-multi-rule
```
