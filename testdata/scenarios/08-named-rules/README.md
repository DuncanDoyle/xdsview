# Scenario 08: Named Rules

One HTTPRoute with three named rules: `a` (`/a`), `a-b` (`/ab`) and `a-matcher-b` (`/amb`). The rule `name` field is in the Gateway API standard channel.

## What to observe

- kgateway appends the rule name to the route name: `...-<ruleIdx>-<matchIdx>-<ruleName>-matcher-<matchIdx>`
- Selecting rule `a` must select neither `a-b` nor `a-matcher-b` (#16)

## K8S resources

- `k8s/gateway.yaml`, `k8s/httpbin.yaml`, `k8s/httproute-kgateway-system_service-httpbin-rg.yaml`: as in 02_7
- `k8s/api-example-com-httproute.yaml`: the three named rules

## Envoy dump

`envoy/kgateway-<version>/config_dump.json`, with `server_info.json`.

## How to collect

```bash
hack/record-dumps.sh v2.5.0 08-named-rules
```
