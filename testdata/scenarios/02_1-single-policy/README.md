# Scenario 02_1: Single Policy — Transformation (TrafficPolicy)

HTTPS Gateway (api.example.com and developer.example.com) with one HTTPRoute and a `TrafficPolicy` that adds a request header through a transformation.

This is the on the first scenarios where we can observe how kgateway translates a K8S policy into an Envoy filter, and what metadata or naming conventions link the two.

## What to observe

- How does the TrafficPolicy appear in the Envoy config? As an HTTP filter, a per-route config, or both?
- Is there any metadata in the Envoy config that references the originating K8S resource?
- How does the filter chain differ from the no-policy baseline (scenario 01)?
- Does the HTTPS listener produce multiple network-level filter chains (e.g., per SNI)?

## K8S resources

- `k8s/httpbin.yaml` — Single rule routing to one backend service
- `k8s/gateway.yaml` — HTTP Gateway on port 80
- `k8s/api_developer-example-com-httproute.yaml` — Single rule routing to one backend service
- `k8s/transformation-trafficpolicy.yaml` — `TrafficPolicy` whose transformation adds `x-season: winter` to requests
- `k8s/httproute-kgateway-system_service-httpbin-rg.yaml` — Reference Grant granting access from the HTTPRoute in the kgateway-system namespace to Services in the httpbin namespace

`k8s/hook-tls-secrets.sh` creates (and on teardown deletes) self-signed TLS secrets `api-example-com` and `developer-example-com` in kgateway-system for the two HTTPS listeners.
The `k8s/create-tls-cert-secret-developer-example-com` creates the `developer-example-com` Secret in the kgateway-system namespace. This secret contains a self-signed certificate for the TLS configuration to api.example.com.

## Envoy dump

HTTPS Gateway (api.example.com and developer.example.com) with one HTTPRoute and a `TrafficPolicy` that adds a request header through a transformation.

## How to collect

```bash
# Record this scenario on a throwaway kind cluster (envoy/kgateway-<version>/)
hack/record-dumps.sh v2.5.0 02_1-single-policy
```
