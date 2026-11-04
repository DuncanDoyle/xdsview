# Scenario 06: Multiple ext_proc filters

HTTP Gateway with two HTTPRoutes on api.example.com (`/` and `/admin`). Each has a `TrafficPolicy` that uses its own `ExtProc` `GatewayExtension`, so the filter chain carries two ext_proc filters, each enabled per route. The extensions point at Services with no pods; no ext_proc server runs.

The route names also test HTTPRoute selection: `api-example-com-admin` extends `api-example-com`, so a filter that matched on a name prefix alone would keep both.

## K8S resources

- `k8s/httpbin.yaml` — HTTPBin backend
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/httproutes.yaml` — HTTPRoutes `api-example-com` (`/`) and `api-example-com-admin` (`/admin`)
- `k8s/ext-proc-a.yaml`, `k8s/ext-proc-b.yaml` — Service (no pods), `ExtProc` `GatewayExtension` and `TrafficPolicy` per route
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — ReferenceGrant from kgateway-system to httpbin namespace

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 06-multi-extproc
```
