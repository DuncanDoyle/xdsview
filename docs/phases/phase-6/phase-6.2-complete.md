# Phase 6.2 — `pkg/envoy/chain`, attribution and narrowing (Complete)

**Completed:** 2026-11-21
**Range:** 45e1f43..5490926 (`main`)

## Summary
`chain.Effective` resolves which HCM filters run for one route, what decided that, and each filter's request and response side. `chain.Annotate` sets that chain on every route of every HCM. With a mapper, it attributes each entry to its extension and to the policy fields that configure it. `filter.Filter` matches annotated routes on their origin and never selects an Ambiguous one, and `filter.Ambiguous` lists those routes. Listeners that are warming, draining, rejected or errored survive narrowing.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Semantics table, levels, precedence, no route config, `Conditional` | `TestEffective_Semantics` (18 cases), `TestEffective_NoRouteConfig` and `TestEffective_Conditional` failed against a stub and pass now. Rules checked against Envoy `per_filter_config.cc` at v1.39.6 (`aacd0340`): `disabled: true` wins over `config`, and a `FilterConfig` with neither field is rejected. |
| Side table and orders | `TestEffective_SidesAndOrders`. Covers a decoder-only filter, an unknown type, and a filter that doesn't run. At Phase 6.2, the table in `chain/sides.go` had 24 types. Each cited the `add*Filter` call at Envoy `aacd0340`, or at envoy-gloo `2fac0b83` for transformation. The Envoy version comes from the recorded bootstrap's `user_agent_build_version` (1.39.6 for kgateway v2.5.0). Four citations were spot-checked. |
| Corpus `03`, v2.4.7 and v2.5.0 | `TestAnnotate_Corpus03`, with literal entries. `global_disable/ext_auth` is off. cors, ext_auth and ratelimit are enabled by route through `CorsPolicy`, `FilterConfig` and `RateLimitPerRoute`. The router is on. The request and response orders are checked. With the kgateway mapper: `cors`, `extAuth` and `rateLimit.global` are attributed, and ext_auth and ratelimit get their GatewayExtension. |
| Corpus `00` | `TestAnnotate_Corpus00`: the router only, on. |
| Contributions: unmapped field, mapper without `PolicyFilters`, plugin in `origin.Chain`, nil mapper, no per-route level | `TestAnnotate_Contributions` and `TestAnnotate_PolicyNeedsPerRouteLevel`. The field table also has its own test, `TestKgateway_PolicyFilter`. |
| `Annotate` copies, and every HCM gets chains | `TestAnnotate_CopiesInput`: two HCMs share a route config, and the input keeps nil chains. |
| `NarrowedByKnown` | `TestRoute_Ambiguity` now asserts it. It failed before the change. |
| Narrowing on origins, `Ambiguous` | `TestFilter_AnnotatedOrigins`: the colliding pair, with and without `Known`, plus rule narrowing. The ambiguous rows failed before the change. The un-annotated rows pass before and after, so name matching is unchanged. |
| Listeners that explain | `TestFilter_ListenersThatExplain`. It failed before the change (no listeners kept). |
| JSON goldens | Only the two `06-multi-extproc` narrowed goldens change: they add `"narrowedByKnown": true` on the admin route. The route name reads two ways, and Known settles it. |
| Boundary | `chain` is in `core`. |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

## Notes for later phases
These are implementation choices the design doesn't state:
1. **`PerFilterType`** is the deciding entry's own `@type`, so it is `FilterConfig` for a wrapper, not the type of the config inside it.
2. **Policies.** `chain.Annotate` asks the mapper for policies (`m.Policies(r)`) rather than reading `Route.Policies`. That way it works on a snapshot that `origin.Annotate` hasn't touched.
3. **Extension contributions** are added whatever the entry's level, because the extension defines the filter instance. Policy contributions need a per-filter `Level`.
4. **A listener with an empty `State.Status`** counts as active for narrowing. Snapshots built by hand or from older JSON behave as before.
5. **Composite filters.** A composite with an unresolved delegate is `yes`/`yes`, because Envoy installs it on both sides. Its delegate may use only one side.

## Deferred / open
none
