# Scenario 04: Multiple Rules

One HTTPRoute with multiple rules targeting different backends (e.g., path-based routing: `/api` → api-svc, `/web` → web-svc).

This scenario tests whether the tool can correctly represent and correlate multiple rules within a single HTTPRoute, each with its own match conditions and backend refs.

## What to observe

- How are multiple rules represented in the Envoy route config? Separate route entries in the same VirtualHost?
- Do per-rule policies (if any) appear as per-route filter configs or as separate filter instances?
- What is the naming pattern for clusters when multiple backends are involved?
- How does the route match config (prefix, headers, etc.) map back to HTTPRoute rule matches?

## K8S resources

- `k8s/gateway.yaml` — HTTP or HTTPS Gateway
- `k8s/httproute.yaml` — Multiple rules with different matches and backends

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 04-multi-rule
```
