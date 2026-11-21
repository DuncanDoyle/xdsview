# Phase 6.5 — Attribution gaps and route hook (Complete)

**Completed:** 2026-11-21
**Range:** b4cf6ab..v0.4.1

## Summary
On OSS kgateway, policy attribution now covers local rate limit, the dynamic-module transformation filter, and the `global_disable/*` helpers that turn ext auth or ext proc off for a route. A malformed or nameless route config costs only itself. `render.Options.RouteExtra` (D-010) gives consumers a route-scoped hook. Found by kgwctl 3.2 on a kgateway v2.5.1 dump (#28).

## Sub-phases (phase record only)
n/a

## Reviews
- `reviews/6.5-codex.md`: Codex, cross-model. 4 findings (2 P1, 1 P2, 1 P3), all accepted and fixed. One note was deferred: other v2.5.1 merge fields are unmapped.

## Waivers
none

## Deliverables

| Item | Test (failed before, passes after) |
|---|---|
| `rateLimit.local` → `ratelimit/local` | `TestAnnotate_Corpus09`, `TestKgateway_PolicyFilter` |
| `transformation` → `dynamic_modules/simple_mutations` (by instance name; the `ext_auth_enabled` helper uses the same module) | `TestAnnotate_Corpus09`, `TestAnnotate_Corpus02_1Transformation`, `TestAnnotate_TransformationBesideAuthMetadata` |
| `extAuth` / `extProc` → `global_disable/ext_auth` / `ext_proc` | `TestAnnotate_Corpus09` (public route), `TestAnnotate_DisableBesideInheritedProvider` |
| Attribution goes to the matching entry at the most specific level | `TestAnnotate_DisableBesideInheritedProvider` (provider at route config, helper at route) |
| Bad, null or nameless route configs are skipped with a warning; the rest are joined | `TestParse_BadRouteConfigSkipped` |
| `RouteExtra` / `RouteRef` (D-010) | `TestText_RouteExtra`, `TestText_RouteExtraRefs`. Mutation: a vhost index hardcoded to 0 fails it. |
| Scenario `09-kgwctl-policies` (kgwctl's v2.5.1 dump, secrets section removed) | Text and JSON goldens added |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

Mappings checked against kgateway v2.5.1 source (`pkg/kgateway/extensions2/plugins/trafficpolicy/`):
- `traffic_policy_plugin.go:45-46`;
- `extauth_policy.go:21-28`;
- `extproc_policy.go:24`;
- `utils.go:82`.

Golden changes:
- `02_1` text and JSON gain the transformation contribution;
- `09` goldens are new;
- no other golden changed.

## Notes for later phases
- **Limit:** a merged field lists every policy that set it, and the dump doesn't say which attachment set which level. The most specific matching entry gets all of them.
- Other v2.5.1 filter-backed merge fields are unmapped: `jwt`, `compression`, `decompression`, `oidc`, `csrf`, `headerModifiers`, `buffer`, `rbac`, `apiKeyAuth`, `basicAuth`, `faultInjection`, `httpACL`. No dump shows them yet.

## Deferred / open
- The unmapped merge fields above (to track on owner OK).
