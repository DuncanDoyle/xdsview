# Scenario 01_3 — RegularExpression Path Match

## What this tests

An HTTPRoute with a single rule using a **RegularExpression** path matcher (`/api/v[0-9]+/.*`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy represents regex path matches using `safe_regex_match` in the route entry. Unlike `prefix` or `path` (exact), a regex match is encoded as:

```json
"match": {
  "safe_regex": {
    "regex": "/api/v[0-9]+/.*"
  }
}
```

The `safe_regex` wrapper is Envoy's protection against regex expressions that could cause catastrophic backtracking (ReDoS). Any regex provided by the Gateway API is passed through into this structure. This is worth observing because it is the least-obvious translation from the Gateway API spec — `RegularExpression` in K8s becomes `safe_regex` in Envoy, not a plain `regex` field.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_3-matchers
```
