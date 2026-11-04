# Scenario 01_8 — Combination: PathPrefix + Multiple Header Matchers

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/api/v2`) with **two** Exact header matchers: `x-api-version: v2` and `x-tenant-id: acme`. No filters. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

This scenario makes Envoy's AND semantics for route matching explicit. All conditions in a single route entry's `match` block are ANDed — a request must satisfy every one of them to match the route. With two headers in the list, the route entry will look like:

```json
"match": {
  "prefix": "/api/v2",
  "headers": [
    {
      "name": "x-api-version",
      "string_match": {
        "exact": "v2"
      }
    },
    {
      "name": "x-tenant-id",
      "string_match": {
        "exact": "acme"
      }
    }
  ]
}
```

A request to `/api/v2/anything` that only carries one of the two headers will not match this route. The `headers` array grows linearly with the number of header matchers defined in the HTTPRoute rule. This is in contrast to OR semantics, which in the Gateway API would require multiple rules (each with its own match block), resulting in multiple Envoy route entries.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_8-matchers
```
