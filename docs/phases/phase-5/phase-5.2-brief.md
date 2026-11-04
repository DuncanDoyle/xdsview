# Sub-phase 5.2 — Model accuracy and corpus

Kind: planned
Base: 6ab2858
Issues: #11, #13, #15, #17 | Plan steps: implementation-plan.md § 5.2

## Problem / goal
The model drops parts of what Envoy runs:
- `prefix_rewrite` (#11)
- `weighted_clusters`, where a split route shows no backend at all (#13)
- router `upstream_http_filters` (#15)
- each filter's type

The OSS corpus also lacks multi-policy, multi-rule, multi-listener, traffic-split and named-rule recordings (#17), and a recording with route-source metadata, which 5.3 needs.

## Expected behaviour

| Input / action | Expected |
|---|---|
| Route with `weighted_clusters` 80/20 | `Route.WeightedClusters` = both, in order; `Cluster` empty; text shows `name (80)` and `name (20)` |
| Route with `prefix_rewrite: /new` | `Route.Rewrite.Prefix` = `/new`; text shows the prefix rewrite |
| Router with upstream filters | `HCMConfig.UpstreamFilters` in order; text shows them after the router |
| Filter wrapped in `ExtensionWithMatcher` | `HTTPFilter.Type` is the inner `@type` |
| Two HCMs sharing one route config | Mutating one's new fields doesn't change the other's |
| Enterprise 06 dump | `UpstreamFilters` = `envoy.filters.http.late_ext_proc`, `envoy.filters.http.upstream_codec` |
| New OSS scenarios on v2.4.7 and v2.5.0 | Filter e2e: exact route names per HTTPRoute and per rule |
| `-o json` | Golden diff adds fields and scenarios only |

## Root cause (if known)
The parser's raw structs never declared these fields (`pkg/envoy/parser/parser.go`, with a TODO for `weighted_clusters`).

## Acceptance tests
- Parser: `accuracy_test.go`, using a synthetic dump, plus e2e assertions on 07 and enterprise 06.
- Renderer: unit tests for the three new displays.
- Filter: e2e tables for 03, 04, 05, 07 and 08.
- CLI: golden files.

## Scope
In:
- `pkg/envoy/model`, `parser`, `filter` (doc wording and e2e only)
- `internal/renderer`
- `hack/record-dumps.sh`
- `testdata/scenarios/{03,04,05,07,08}*`, the 04 route-source variant, `testdata/golden`

Out: `origin` (5.3), `--rule <name>` (5.3 and 5.4), and showing origins in the CLI (5.4).

## Plan / design impact
None.

## Definition of done
- [ ] acceptance tests fail before, pass after
- [ ] full test suite passes
- [ ] phase-5.2-complete.md written; phase-status.md updated; #11, #13, #15, #17 closed when the branch reaches main
