# Phase status

## Phase
Phase: 7 — Intended vs Accepted Configuration
Base: n/a (not started)
Docs: docs/phases/phase-7/
Status: not started. Phase 6 complete 2026-11-21 (`v0.4.0`, docs/phases/phase-6/phase-6-complete.md). Starts when a consumer needs it (E4 second half).

### Queue
- [ ] Q1 Phase 7 design + plan

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [ ] design+plan cross-model review + triage
- [ ] phase cross-model review + triage
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-7-complete.md written, roadmap updated, tracked items closed

## Sub-phase
Sub-phase: 6.7 — Silent HCM skips, listeners mid-update (stabilization, `v0.4.3`)
Base: ab593d2
Brief: docs/phases/phase-6/phase-6.7-brief.md
Issues: #31

### Queue
- [ ] Q1 (P1) warning for an HCM that fails to decode ← CURRENT
- [ ] Q2 (P2) narrowing keeps listeners with `Updating` or `Draining`
- [ ] Q3 Codex review of `ab593d2..HEAD`, triage
- [ ] Q4 close: complete record, `v0.4.3`, #31, kgwctl

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`, updated at `3b71687`)
Waiting on: nothing
Consumers: E11 available (`v0.4.2`, kgwctl#10); E4 effective chain available, intended vs accepted is Phase 7 (snellius#63)

## Carry-over (deferred to later phases)
- #26 (P2) kube source: cancellation through an API-server proxy — deferred, not a requirement (owner, 2026-11-05). kgwctl fixed the same gap in its own dialer and offered its code.
- Phase 6 review #13: Phase 7 must compare listeners on `Listener.State` (roadmap Phase 7).
- #32 E6 cleanup (enterprise corpus, enterprise type in the core mapper): its own phase, before kgwctl moves into kgateway (owner, 2026-11-21).

## Notes
- 2026-11-21: Phase 6 patch `v0.4.2` (sub-phase 6.6, #29): narrowing edge cases. #30 open (unmapped policy fields).
- 2026-11-21: Phase 6 patch `v0.4.1` (sub-phase 6.5, #28, D-010): attribution gaps and `RouteExtra`.
- 2026-11-21: Phase 6 closed: `v0.4.0` tagged, #27 closed, notices kgwctl#10 and snellius#63, program file `3b71687`.
- 2026-11-20: owner started Phase 6 as the E11 route view (D-008); intended vs accepted moved to Phase 7.
- 2026-11-05: library renamed to `xdsview` (D-007, `v0.3.0`).
- 2026-11-05: route origins without `--route` decided as option B (D-006): live mode lists HTTPRoutes. Shipped in `v0.2.1`.
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
