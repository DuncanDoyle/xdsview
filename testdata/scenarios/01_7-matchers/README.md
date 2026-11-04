# Scenario 01_7 — Query Parameter Matcher (Exact Value)

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/search`) with an **Exact** query parameter matcher (`format=json`). No filters. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy has a dedicated `query_parameters` array in the route match config for query parameter matching. For an exact value match, look for:

```json
"match": {
  "prefix": "/search",
  "query_parameters": [
    {
      "name": "format",
      "string_match": {
        "exact": "json"
      }
    }
  ]
}
```

Query parameter matchers in Envoy are parallel in structure to header matchers — they sit alongside the path condition and are ANDed together. One thing to note is that query parameters are matched against the raw query string, so `format=json` in the URL must match exactly (including case) for the route to be selected.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_7-matchers
```
