# Scenario 05: Multiple Listeners

Gateway with both HTTP (port 80) and HTTPS (port 443) listeners. HTTPRoute uses `sectionName` in its `parentRef` to target a specific listener.

This scenario tests listener selection logic and validates that the tool picks the correct listener based on the HTTPRoute's parentRef.

## What to observe

- Does each listener produce a separate Envoy listener in the config dump?
- How are the listener names structured (e.g., `0.0.0.0_80`, `0.0.0.0_443`)?
- Does the HTTPS listener have multiple network-level filter chains (per SNI)?
- How does `sectionName` on the parentRef map to the Envoy listener?
- If an HTTP→HTTPS redirect is configured, where does that appear?

## K8S resources

- `k8s/gateway.yaml` — Gateway with HTTP (80) and HTTPS (443) listeners
- `k8s/httproute.yaml` — HTTPRoute with `parentRef.sectionName` targeting the HTTPS listener

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 05-multi-listener
```
