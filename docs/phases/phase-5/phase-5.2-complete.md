# Phase 5.2 — Model accuracy and corpus (Complete)

**Completed:** 2026-11-05
**Range:** cf2e851..08ec3e2 (branch `duncandoyle/phase5`, pending design approval)

## Summary
The model now captures:
- traffic splits (`Route.WeightedClusters`)
- prefix rewrites (`Route.Rewrite.Prefix`)
- router upstream filters (`HCMConfig.UpstreamFilters`)
- each HTTP filter's type (`HTTPFilter.Type`)

The text view shows all four. The OSS corpus gains five scenarios on kgateway v2.4.7 and v2.5.0: multi-policy, multi-rule, multi-listener, traffic split and named rules. 04 also has a recording with route-source metadata.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Weighted clusters, prefix rewrite, upstream filters, types | `pkg/envoy/parser/accuracy_test.go`. It didn't compile before the change (`undefined: model.WeightedCluster`) and passes now. |
| Shared route config stays isolated | `TestParse_SharedRouteConfigIsolated` |
| Composite filter type | `TestParse_CompositeFilterType`: same-type actions take the delegate type; mixed types stay `Composite` |
| Real dumps | `TestE2E_TrafficSplit` (07, both versions), `TestE2E_PolicyFilterTypes` (OSS 02_1, 02_7, 02_8, 06), `TestE2E_UpstreamFilters` (enterprise 06: `late_ext_proc`, then `upstream_codec`, parsed and rendered in order) |
| Renderer | `internal/renderer/accuracy_test.go`: three tests that failed before the change and pass now |
| Corpus | 03, 04, 05, 07, 08 on v2.4.7 and v2.5.0, plus 04 `-route-source` on both. Filter e2e uses exact route names per rule. `TestFilter_E2E_OSSCorpusComplete` failed until the tables were added. |
| Golden files | 138 lines added, 0 removed; 5 new scenario directories |
| tests / vet | The staged set alone and the full tree: `go test -count=1 ./...` ok; `go vet ./...` clean |

## Notes for later phases
- **`route_source` differs by version.** v2.5.0 writes `rule: "_rule-<idx>"` (or the rule name) and `match: "_match-<idx>"`. v2.4.7 writes `rule: "httproute-<name>-<ns>-<rule>-<match>"` and no `match`. The source reading for the design missed this; the recordings show it.
- **kgateway's ext_proc filters** are `ExtensionWithMatcher` → `Composite`, with the `ExternalProcessor` inside the matcher's `ExecuteFilterAction`. Unwrapping only the matcher would report `composite`. Design § Model now says this.
- **OSS routers have no upstream filters.** The `upstream_codec` in OSS dumps belongs to cluster-level HTTP filters, which krp doesn't model.
- **Recording.** Two runs failed and were re-run:
  - 04 on v2.5.0: `scenario.sh` waited on an httpbin pod label before the pod existed. Fixed with `rollout status`.
  - 03 on v2.4.7: the proxy wasn't Available within 180s. This was the first scenario on a new cluster, most likely the image pull; now 5m with pod diagnostics.
  - 05 on v2.5.0 once didn't settle in 120s, then settled on the re-run. The cause is unknown; the script now saves two reads for diffing if it recurs.
- **Ordering.** Route order `<N>` follows precedence: in 08, `/amb` (`a-matcher-b`) is route-0, `/ab` route-1 and `/a` route-2.

## Deferred / open
- #11, #13, #15, #17 to be closed when this branch reaches `main`.
