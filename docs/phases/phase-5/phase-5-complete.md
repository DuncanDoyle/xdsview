# Phase 5 — Data-Plane Accuracy and Mapping (Complete)

**Completed:** 2026-11-05
**Range:** badf0ab..v0.2.0

## Summary
- **Cancellation.** Cancelling a kube-source call stops the port-forward, so the kgateway-ui backend can use `source/kube` (#25). The exception is an API-server proxy (#26).
- **Accuracy.** The model captures traffic splits, prefix rewrites, router upstream filters and each filter's type.
- **Mapping.** `pkg/envoy/origin` maps Envoy objects back to the Gateway API objects that produced them (E5). It's pluggable for vendor kinds (P-002) and serves the UI's `EnvoyNamer`.
- **CLI.** `krp dump` shows those origins, and `--rule` takes a rule name.
- **Corpus.** The OSS corpus covers multi-policy, multi-rule, multi-listener, traffic split and named rules on kgateway v2.4.7 and v2.5.0.

## Sub-phases (phase record only)
| Sub-phase | Kind | Issues | Description |
|---|---|---|---|
| 5.1 | planned | #25 | Context-aware port-forward dial (`phase-5.1-complete.md`) |
| 5.2 | planned | #11, #13, #15, #17 | Model fields, renderer, five new scenarios on both versions, route-source recordings (`phase-5.2-complete.md`) |
| 5.3 | planned | #16, E5 | `pkg/envoy/origin`; `filter.Options.RuleName` (`phase-5.3-complete.md`) |
| 5.4 | planned | #16 | CLI origins and `--rule <name>` (`phase-5.4-complete.md`) |

## Reviews
- `reviews/design-codex.md`: 10 findings, 10 accepted
- `reviews/phase-codex.md`: 9 findings, 8 accepted and fixed, 1 deferred (#26)
- `reviews/fixes-codex.md`: 3 findings, 3 accepted and fixed (one in part, see triage)

## Waivers
- Process, not a gate: the owner was away, so 5.1–5.4 were built on branch `duncandoyle/phase5` before the design was approved. The owner approved on 2026-11-05 ("Approved the design"), and the branch was merged unchanged.

## Deliverables
- `pkg/envoy/origin`: `Mapper`, `Kgateway`, `Chain`, `Annotate`, `ObjectForName`.
- `pkg/envoy/model`: the origin types; `WeightedClusters`, `Rewrite.Prefix`, `UpstreamFilters`, `HTTPFilter.Type`.
- `pkg/envoy/source/kube`: the dial honours ctx, verifies TLS on every path, and closes its connections on teardown.
- `-o json`: origin and policy fields added; still `v1alpha1`.
- Corpus: scenarios 03, 04 (plus `-route-source`), 05, 07, 08 on both versions; `hack/route-source-values.yaml`.
- Live kind checks of `krp dump --gateway` after each change to the kube source.

## Notes for later phases
- **The two versions write different `route_source` metadata.** v2.4.7 writes `rule: "httproute-<name>-<ns>-<r>-<m>"`; v2.5.0 writes `_rule-N` (or the rule name) and `_match-N`. Both are decoded.
- **Route names are ambiguous.** A route name usually has several readings. A route's HTTPRoute needs `Known` refs or route-source metadata to be certain.
- **kgateway's ext_proc filters** are `ExtensionWithMatcher` → `Composite`. Their type comes from the `ExecuteFilterAction`.
- **Phase 6 boundary.** The intended-vs-accepted comparison ignores origin fields; they only label the differences (design § Phase 6 boundary).

## Deferred / open
- #26 (P2): cancellation through an API-server proxy. Not a requirement for now (owner, 2026-11-05).
- Owner decision carried into Phase 6: how the CLI shows route origins without `--route` (options A–E in phase-status Notes).
