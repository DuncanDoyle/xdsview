# Sub-phase 6.2 — `pkg/envoy/chain`, attribution and narrowing

Kind: planned
Base: 45e1f43
Issues: #27 (E11) | Plan steps: implementation-plan.md § 6.2

## Problem / goal
6.1 put the inputs in the model. 6.2 computes each route's effective filter chain and attributes its entries to policies and extensions. It also makes `filter.Filter` narrow on resolved origins and keep listeners that explain a missing route.

## Expected behaviour

**`chain.Effective(hcm, vh, r)`**, per HCM filter `f`. The deciding entry is `typed_per_filter_config[f.Name]` at route, else vhost, else route config.

| HCM `disabled` | Deciding entry | `Runs` | `State` | `Level` |
|---|---|---|---|---|
| false | none | true | on | "" |
| false | `FilterConfig{disabled: true}` (with or without `config`) | false | disabled | its level |
| false | `FilterConfig{config: …}`, or a filter-specific config | true | on | its level |
| true | none | false | off | "" |
| true | `FilterConfig{disabled: true}` | false | off | its level |
| true | `FilterConfig{config: {}}`, or a filter-specific config | true | enabled | its level |
| any | `FilterConfig` with neither `disabled: true` nor `config` | false | unknown | its level |

- `PerFilterType` is the deciding entry's `@type`.
- A route entry beats a vhost entry, which beats a route config entry. An HCM without a route config uses route and vhost only.
- `Request` and `Response` come from a table of filter types, cited from Envoy v1.39.6 (`aacd0340`, the version kgateway v2.5.0 ships, read from the recorded bootstrap). A type not in the table gets `unknown` on both sides.
- `Conditional` is true when any backend in `r.WeightedClusters` has per-filter config.
- Filters keep HCM order. Every HCM filter has an entry.

**Corpus** (`03-multi-policy` on v2.4.7 and v2.5.0, `00-simple`). The literal expected entries come from the dumps and are written into the test, not computed.

**`chain.Annotate(snap, m)`** returns a copy with `Route.Chain` set on every route in every HCM, and leaves the input unchanged. With a mapper that implements `origin.PolicyFilters`:

| Case | `Contributions` on the entry |
|---|---|
| `03`: entries enabled per route by `extAuth`, `rateLimit.global` and `cors` | that policy and field. The ext_auth entry also gets its GatewayExtension (`kind: extension`). |
| a policy field with no mapping | nothing |
| a mapper without `PolicyFilters` | extension contributions only |
| `origin.Chain(plugin, Kgateway)`, where the plugin maps its own field | the plugin's field is attributed |
| an entry without a per-route `Level` | no policy contribution |

**Narrowing (`filter.Filter`)**

| Snapshot / route | Expected |
|---|---|
| route with `Origin` Exact, or Inferred with one candidate | selected when the candidate's HTTPRoute (and rule, if asked) match |
| route with `Origin` Ambiguous | never selected. `filter.Ambiguous(snap, opts)` lists its name when a candidate is the requested HTTPRoute. |
| colliding pair `orders`/`team-a` and `orders-team`/`a`, without `Known` | both routes are Ambiguous: neither is selected, and both are listed by `Ambiguous` |
| same pair with `Known` = only `orders`/`team-a` | the route resolves to `orders`/`team-a`, is selected, and has `NarrowedByKnown: true` |
| route with no `Origin` | unchanged from today: `route_source` metadata, else the name |
| warming, draining or rejected listener, or active with `Error`, and no matching route | kept, with its state. A rejected listener has no chains. |
| active listener without `Error` and no matching route | dropped, as today |

## Root cause (if known)
n/a (new behaviour).

## Acceptance tests
- `pkg/envoy/chain`: a table test with one synthetic case per row above, covering levels, precedence, no route config and `Conditional`. It asserts literal `Runs`, `State`, `Level`, `PerFilterType`, `Request`, `Response` and both orders. Corpus tests through `parser.Parse` on the recorded dumps.
- `pkg/envoy/chain`: `Annotate` tests with `origin.Kgateway` on `03`, a field-less mapper, and a plugin through `origin.Chain`, plus the immutability check.
- `pkg/envoy/origin`: `NarrowedByKnown` on the colliding pair.
- `pkg/envoy/filter`: annotated snapshots for each narrowing row. The existing tests on un-annotated snapshots stay unchanged.
- `pkg/envoy`: the boundary test includes `chain`.

## Scope
In: `pkg/envoy/chain` (new), `pkg/envoy/origin`, `pkg/envoy/filter`, `pkg/envoy/model` (`NarrowedByKnown`), the boundary test.
Out: `render`, `krp` and the TUI (6.3).

## Plan / design impact
None.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] `go test ./...` and `go vet ./...` pass
- [ ] cross-model review at the phase review (planned sub-phase)
- [ ] phase-6.2-complete.md written; phase-status.md updated; kgwctl messaged
