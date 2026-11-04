# Scenario 03: Multiple Policies

HTTPS Gateway with one HTTPRoute and multiple policies attached — e.g., auth (ext_authz or JWT), rate limiting, and a transformation.

This scenario tests the Correlator's ability to match multiple Envoy HTTP filters back to their respective K8S policy resources.

## What to observe

- Do all policies produce distinct HTTP filters, or do some share a filter?
- What is the ordering of filters in the HCM — does it match the order policies were applied?
- Can each filter be individually correlated back to its K8S policy?
- Are there any per-route filter configs vs listener-level filter configs?

## K8S resources

- `k8s/gateway.yaml` — HTTPS Gateway
- `k8s/httproute.yaml` — Single rule with one backend
- `k8s/route-option.yaml` — RouteOption with transformation
- `k8s/virtual-host-option.yaml` — VirtualHostOption with rate limiting or auth

## Envoy dump

- `envoy/kgateway-<version>/config_dump.json` — `/config_dump` per kgateway version, with `server_info.json`

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 03-multi-policy
```
