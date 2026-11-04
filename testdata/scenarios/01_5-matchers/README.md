# Scenario 01_5 — Header Matcher (RegularExpression Value)

## What this tests

An HTTPRoute with a single rule combining a **PathPrefix** matcher (`/api`) with a **RegularExpression** header matcher (`x-client-id` matching `[a-z]+-[0-9]+`). No filters are applied. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

For a regex header match, Envoy uses `safe_regex` inside the header matcher, just as it does for regex path matches. Look for:

```json
"match": {
  "prefix": "/api",
  "headers": [
    {
      "name": "x-client-id",
      "string_match": {
        "safe_regex": {
          "regex": "[a-z]+-[0-9]+"
        }
      }
    }
  ]
}
```

The key observation is consistency: Envoy always uses `safe_regex` for regex matching, whether the match target is a path, a header, or a query parameter. This wrapping is a Envoy-level safety mechanism, not something visible in the Gateway API spec itself.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_5-matchers
```
