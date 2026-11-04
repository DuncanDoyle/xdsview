# Phase status

## Phase
Phase: 5 — Data-Plane Accuracy and Mapping
Base: badf0abb9481f4b8d80b63184093f6b434bc9ecf
Docs: docs/phases/phase-5/
Status: design in progress (2026-11-04). Phase 4 complete 2026-11-04, tag `v0.1.0` (`docs/phases/phase-4/phase-4-complete.md`).

### Queue
- [x] Q1 Phase 5 design (`design.md`)
- [x] Q2 Phase 5 implementation plan (`implementation-plan.md`): planned sub-phases 5.1–5.4
- [ ] Q3 Cross-model review of design + plan (10 accepted: reviews/design-codex.md), owner approval ← CURRENT (pending: owner is away; 5.x work happens on branch `duncandoyle/phase5` until approval)
- [x] Q4 Sub-phase 5.1: #25 cancellation (`a0c7cc9`, phase-5.1-complete.md; #25 closes on merge)
- [x] Q5 Sub-phase 5.2: model accuracy (#11, #13, #15) and corpus (#17) (`022ea2c`, `08ec3e2`, phase-5.2-complete.md; issues close on merge)
- [x] Q6 Sub-phase 5.3: `pkg/envoy/origin` (E5) and `--rule` by name in `filter` (#16) (`accf3a7`, phase-5.3-complete.md)
- [ ] Q7 Sub-phase 5.4: CLI origins, `--rule <name>`, golden files
- [ ] Q8 Phase review (cross-model) and triage
- [ ] Q9 Phase close: `v0.2.0`, program file, cross-repo notices

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
Waiting on: none (program file checked 2026-11-04 at `ece05e7`; no open `cross-repo` issues in krp)
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
