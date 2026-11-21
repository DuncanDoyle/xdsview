# Scenario 09: kgwctl policies

A kgateway v2.5.1 dump (Envoy 1.39.6), recorded by kgwctl from its live proxy fixtures (`hack/live/proxy-fixtures.yaml` in DuncanDoyle/kgwctl), in namespace `kgwctl-proxy`. The `SecretsConfigDump` section was removed; xdsview doesn't read it.

## What to observe

- HTTPRoute `orders`: TrafficPolicies set `transformation` (`dynamic_modules/simple_mutations`), `extAuth` (`ext_auth/kgwctl-proxy/authz`) and `rateLimit.local` (`ratelimit/local`), each enabled per route.
- HTTPRoute `public`: a TrafficPolicy disables ext auth by enabling `global_disable/ext_auth` per route.

There are no `k8s/` manifests here, so `hack/record-dumps.sh` skips this scenario. Re-record it from kgwctl.
