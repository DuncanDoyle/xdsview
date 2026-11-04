# Phase status

## Phase
Phase: 5 — Data-Plane Accuracy and Mapping
Base: n/a (recorded at phase start)
Docs: docs/phases/phase-5/
Status: not started. Phase 4 complete 2026-11-04, tag `v0.1.0` (`docs/phases/phase-4/phase-4-complete.md`).

### Queue
- [ ] Q1 Phase 5 design (`design.md`) ← CURRENT

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [ ] design+plan cross-model review + triage: docs/phases/phase-5/reviews/design-codex.md
- [ ] phase cross-model review + triage: docs/phases/phase-5/reviews/phase-codex.md
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-5-complete.md written, roadmap updated, tracked items closed

## Sub-phase
none

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`)
Waiting on: to be checked at phase start
Consumers: E5 (Envoy → Gateway API mapping); E4 needs the Q9 fix before kgateway-ui's backend uses the kube source

## Carry-over (deferred to later phases)
- #25 (Q9, P2) kube source keeps a port-forward connection open when a call is cancelled mid-connect — from Phase 4 review (waiver in phase-4-complete.md); fix before kgateway-ui's backend uses the kube source
- #11 (P2) `prefix_rewrite` route action not captured
- #13 (P3) `weighted_clusters` not captured (traffic splitting)
- #15 Support HTTPRoute filters
- #16 RFE: `--rule <name>` when the HTTPRoute rule name reaches the standard channel, if promoted
- #17 Collect multi-route config dumps for E2E filter tests (03, 04, 05)

## Notes
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
