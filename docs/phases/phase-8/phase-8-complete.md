# Phase 8 — Vendor-Neutral Core (Complete)

**Completed:** 2026-11-23
**Range:** f8b4af6..c1348fb (released as `v0.6.0` at 10512f6)

## Summary
Every attribution now states how it was derived. A policy is linked to a filter only when exactly one filter configured for the route matches it, and only for kgateway's TrafficPolicy. The downstream dumps, goldens and transformation type left the module; neutral hand-built fixtures keep the coverage they gave. `v0.5.0` delivered E13 (certainty); this release delivers E6's cleanup. OSS output is byte-for-byte the same as `v0.5.0`.

## Sub-phases (phase record only)
| Sub-phase | Kind | Issues | Description |
|---|---|---|---|
| 8.1 | planned | #32 | Certainty on contributions, policy refs and route indices; one-candidate link (`v0.5.0`) |
| 8.2 | planned | #32 | Downstream corpus out (D-018); `upstream-extproc` fixture |
| 8.3 | planned | #32 | Policy links only for TrafficPolicy; vendor transformation type out |
| review fixes | stabilization | phase review, four fix rounds | Evidence checks on route-name readings, policy IDs and the transformation link |

## Reviews
- `reviews/design-codex.md`: design and plan, triaged.
- `reviews/8.1-codex.md`, `8.1-fixes-codex.md`: 8.1 before `v0.5.0`.
- `reviews/phase-codex.md`: 6 findings: 5 accepted, 1 deferred to P-009.
- `reviews/phase-fixes-codex.md` to `phase-fixes4-codex.md`: 4, 4, 6 and 5 findings. All accepted except three in round 4, deferred by the owner (see Waivers).

## Waivers
Fix round 4 findings #2, #4 and #5 (P1 correctness) are deferred to Phase 9 instead of fixed. The owner chose "Defer the 3, close" (2026-11-23). None occurs in the recorded OSS corpus, and Phase 9 replaces this attribution code (P-010).

## Deliverables
- `model`: `Certainty` on `Contribution` and `PolicyRef`; `RouteOrigin.IndexCertainty` (additive).
- Text output marks any attribution that isn't exact.
- `origin`:
  - `PolicyFilter` links only `gateway.kgateway.dev/TrafficPolicy`;
  - `Policies` skips IDs without a valid identity;
  - a route-name reading fills only fields that agree with `route_source`.
- The former downstream corpus directory and its goldens removed; `testdata/fixtures/upstream-extproc` added.
- Decisions D-012 to D-019. No new dependencies.

## Notes for later phases
- Phase 9 (P-010, D-019) replaces the mapper-driven attribution with per-claim evidence. The deferred findings below belong in its design.
- Text and JSON goldens change only through reviewed `-update` runs. OSS goldens did not change from `v0.5.0`.

## Deferred / open
- Fix round 4 #2: a dotted rule name (`api.v1`) gets no index from the Envoy name, so `--rule <n>` misses the route.
- Fix round 4 #4: `dynamic_module_config.module` can override the library name the transformation check reads.
- Fix round 4 #5: a Composite-wrapped rustformation filter loses its inferred link.
- Phase review #6: keep `version_info` and `version_text` distinct (P-009 investigation).
- #26, #30, `ObjectForName` certainty: carried over.
