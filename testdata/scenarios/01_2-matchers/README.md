# Scenario 01_2 — Exact Path Match

## What this tests

An HTTPRoute with a single rule using an **Exact** path matcher (`/api/v1/users`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

Envoy represents an Exact path match differently from a prefix match. In the route config, look for a route entry using `path` (exact string comparison) rather than `prefix` or `safe_regex`. The route entry should look like:

```json
"match": {
  "path": "/api/v1/users"
}
```

This is in contrast to a PathPrefix match, which uses `"prefix": "/api/v1"`, or a regex match which uses `"safe_regex"`. Observing the difference in these three scenarios (01_1, 01_2, 01_3) makes the Gateway API-to-Envoy path type translation explicit.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_2-matchers
```
