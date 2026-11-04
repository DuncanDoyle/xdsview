# Scenario 01_4 — Header Matcher (Exact Value)

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/api`) with an **Exact** header matcher (`x-api-version: v1`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy encodes header matchers as a `headers` array alongside the path match condition in the route entry. For an Exact header match, look for:

```json
"match": {
  "prefix": "/api",
  "headers": [
    {
      "name": "x-api-version",
      "string_match": {
        "exact": "v1"
      }
    }
  ]
}
```

All conditions in the `match` block are ANDed together — a request must match both the path prefix and the header value to be routed. This makes header matchers a good way to demonstrate Envoy's multi-condition AND semantics in route matching.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_4-matchers
```
