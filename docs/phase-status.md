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
Sub-phase: 6.5 — Attribution gaps and route hook (`v0.4.1`, Phase 6 patch)
Kind: stabilization (plus D-010, owner-approved 2026-11-21)
Base: b4cf6ab
Brief: docs/phases/phase-6/phase-6.5-brief.md

### Allowed work
Bug fixes, missing designed behaviour, tests, docs; `render.Options.RouteExtra` (D-010).

### Not allowed
Other new features, architecture changes, new dependencies.

### Queue
- [ ] S1 (P2) `rateLimit.local`, OSS transformation, `global_disable/*` attribution — #28 ← CURRENT
- [ ] S2 (P3) Per-route-config parse skipping — #28
- [ ] S3 `RouteExtra` (D-010)
- [ ] S4 Cross-model review, triage; `v0.4.1`; close #28; tell kgwctl

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`, updated at `3b71687`)
Waiting on: nothing
Consumers: E11 available (`v0.4.0`, kgwctl#10); E4 effective chain available, intended vs accepted is Phase 7 (snellius#63)

## Carry-over (deferred to later phases)
- #26 (P2) kube source: cancellation through an API-server proxy — deferred, not a requirement (owner, 2026-11-05). kgwctl fixed the same gap in its own dialer and offered its code.
- Phase 6 review #13: Phase 7 must compare listeners on `Listener.State` (roadmap Phase 7).

## Notes
- 2026-11-21: Phase 6 closed: `v0.4.0` tagged, #27 closed, notices kgwctl#10 and snellius#63, program file `3b71687`.
- 2026-11-20: owner started Phase 6 as the E11 route view (D-008); intended vs accepted moved to Phase 7.
- 2026-11-05: library renamed to `xdsview` (D-007, `v0.3.0`).
- 2026-11-05: route origins without `--route` decided as option B (D-006): live mode lists HTTPRoutes. Shipped in `v0.2.1`.
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
