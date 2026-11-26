# Scenario 10: Policy levels

A kgateway v2.5.2 dump (Envoy 1.39.6, Gateway API v1.7.0) from a kind cluster, installed with `hack/route-source-values.yaml`. Three TrafficPolicies target the Gateway, one of its listeners, and an HTTPRoute.

## What to observe

- `tp-gateway` targets the Gateway: kgateway writes its `merge.*` record at listener and at route config level, on both listeners.
- `tp-listener-http` targets listener `http`: its record is on that listener's virtual host.
- `tp-route-http` targets HTTPRoute `route-http`: route-level `rateLimit.local` and `headerModifiers`.
- On `route-http`, CORS is decided at vhost level and the local rate limit at route level, so the less specific records are overridden.

## K8S resources

`manifest.yaml`: namespace `demo`, the `echo` backend, Gateway `gw` with listeners `http` (8080) and `admin` (8081), one HTTPRoute per listener, and the three TrafficPolicies.

The manifest uses its own namespace and Gateway, not the `httpbin` and `kgateway-system` layout of `hack/scenario.sh`, so it sits outside `k8s/` and `hack/record-dumps.sh` skips this scenario. To re-record, apply `manifest.yaml` and save `/config_dump` and `/server_info` from the `gw` proxy in namespace `demo`.

## Envoy dump

`envoy/kgateway-v2.5.2-route-source/config_dump.json`, with `server_info.json`.
