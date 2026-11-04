# Scenario 01_6 — Method Matcher

## What this tests

An HTTPRoute with a single rule using only a **Method** matcher for `GET`. No path matcher is specified (so it matches all paths). No filters. Traffic is routed to the httpbin backend.

## What to observe in the Envoy config dump

This is one of the more interesting translations to observe. The Gateway API has a first-class `method` field on HTTPRouteMatch, but HTTP methods are not a first-class match type in Envoy's route config. Instead, Envoy translates the method matcher into a **header match on the pseudo-header `:method`**:

```json
"match": {
  "prefix": "/",
  "headers": [
    {
      "name": ":method",
      "string_match": {
        "exact": "GET"
      }
    }
  ]
}
```

The `:method` header is an HTTP/2 pseudo-header that is also used by Envoy for HTTP/1.1 method matching internally. This translation is invisible to the end user at the Gateway API level but becomes visible when inspecting the Envoy route config — a good example of where the abstraction layers diverge.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_6-matchers
```
