# Scenario 01_9 — Combination: Exact Path + Method + Query Parameter

## What this tests

An HTTPRoute with a single rule combining three match types: an **Exact** path matcher (`/api/v1/search`), a **Method** matcher (`GET`), and an **Exact** query parameter matcher (`q=hello`). No filters. Traffic is routed to the httpbin backend.

This is the most complex matcher scenario in the 01_x series and is designed to show how Envoy ANDs together all three condition types in a single route entry.

## What to observe in the Envoy config dump

The resulting Envoy route entry pulls together all three translation patterns covered by earlier scenarios: exact path as `path`, method as a `:method` header match, and a query parameter matcher. Look for:

```json
"match": {
  "path": "/api/v1/search",
  "headers": [
    {
      "name": ":method",
      "string_match": {
        "exact": "GET"
      }
    }
  ],
  "query_parameters": [
    {
      "name": "q",
      "string_match": {
        "exact": "hello"
      }
    }
  ]
}
```

This is the most restrictive route in the series — a request must have exactly the right path, the right method, and the right query parameter to match. Comparing this route entry to 01_1 (prefix only, no additional conditions) shows clearly how each Gateway API match type is appended as an additional AND condition in Envoy's match block. The method-as-header translation (`:method`) is particularly notable since it is not obvious from the Gateway API spec that methods end up in the `headers` array rather than a dedicated `method` field.

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 01_9-matchers
```
