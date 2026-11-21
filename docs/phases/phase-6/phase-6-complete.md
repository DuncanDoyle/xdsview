# Phase 6 — Route View for Consumers (Complete)

**Completed:** 2026-11-21
**Range:** 40a5863..v0.4.0

## Summary
xdsview now answers which HTTP filters run for one route, and in what order. `chain.Effective` and `chain.Annotate` resolve each route's effective chain from the HCM filters and per-filter config. Each entry is attributed to its extension and policy fields. Listener state (active, warming, draining, rejected, and errors) is in the model. `pkg/envoy/render` prints the route view as deterministic plain text with no styling dependency. `krp` and its TUI style that output. `filter.Filter` narrows annotated snapshots on their resolved origins. This delivers program edge E11 (kgwctl Phase 3.2, #27), and the same model serves E4 (Snellius Phase 12). The live kind check on scenario `03` matched the effective chain with Envoy's ext_authz stats.

## Sub-phases (phase record only)
| Sub-phase | Kind | Issues | Description |
|---|---|---|---|
| 6.1 | planned | #27 | Model and parser: listener state, per-filter config at vhost, route config and backend |
| 6.2 | planned | #27 | `pkg/envoy/chain`, policy attribution, narrowing on origins |
| 6.3 | planned | #27 | `pkg/envoy/render`; `krp` and the TUI over it; live kind check |
| 6.4 | stabilization | phase review #1–#12, #14 | Phase review fixes |

## Reviews
- `reviews/design-codex.md`: 12 findings (incl. 3 from kgwctl), all accepted (one partly).
- `reviews/phase-codex.md`: 14 findings: 13 accepted, 1 deferred to Phase 7.
- `reviews/6.4-codex.md`: 8 findings, all accepted.

## Waivers
none

## Deliverables
- New packages `pkg/envoy/chain` and `pkg/envoy/render`, both in the boundary test's `core`, with no charmbracelet dependency.
- `model`:
  - `ListenerState`, `EffectiveChain` and related types;
  - `TypedPerFilterConfig` on `RouteConfig`, `VirtualHost` and `WeightedCluster`;
  - `RouteAnnotation.NarrowedByKnown`.
- `origin.PolicyFilters` (optional), and `filter.Ambiguous`.
- `krp -o json` adds `state` and `effectiveChain` (`v1alpha1`).
- Golden `.txt` files for every OSS corpus dump.
- No new dependencies.
- Decisions D-008 and D-009.

## Notes for later phases
- **Breaking (Go):** `model.WeightedCluster` isn't comparable (D-009).
- `krp` narrows before annotating, so its `--route` keeps name matching.
- The side table (`chain/sides.go`) cites Envoy v1.39.6 (`aacd0340`). Re-check it when kgateway moves Envoy minor.
- Padding counts runes; East Asian wide characters still count as one column.
- Phase 7: compare listeners using `Listener.State` (phase review #13, in the roadmap).

## Deferred / open
- Phase review #13: Phase 7 design.
- #26: kube source cancellation through an API-server proxy (carry-over). kgwctl offered its dialer code.
