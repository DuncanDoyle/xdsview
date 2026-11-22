# Phase status

## Phase
Phase: 8 — Vendor-Neutral Core
Base: f8b4af6
Author: anthropic (claude-opus-5-5)
Advisory: none
Docs: docs/phases/phase-8/
Status: design + plan reviewed (Codex, triaged); reordered certainty-first (D-015); approved (D-016). Phase 7 deferred (D-011, docs/phases/phase-7/assessment.md).

### Queue
- [x] Q1 design + plan cross-model review, triage, owner approval (D-016)
- [x] Q2 8.1 certainty and the filter link (D-015): 04c5e07, a3cfd77, 355068a
- [x] Q2b review of 8.1, tag `v0.5.0`; kgwctl notice DuncanDoyle/kgwctl#11 (E13)
- [x] Q3 8.2 corpus out, OSS coverage kept (D-018)
- [ ] Q4 8.3 mapper vendor-neutral ← CURRENT
- [ ] Q5 phase review, close, `v0.6.0`, handoff (E6)

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [ ] design+plan cross-model review + triage: docs/phases/phase-8/reviews/design-codex.md
- [ ] phase cross-model review + triage
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-8-complete.md written, roadmap updated, tracked items closed

## Sub-phase
none (8.2 closed 2026-11-22)

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`, updated at `7288049`)
Waiting on: nothing. Phase 8 needs no edge from another repo.
Consumers: E6 (kgwctl move into kgateway) waits on this phase. E11 available (`v0.4.3`). E4: effective chain available; intended vs accepted deferred (D-011), replacement is the program's call.

## Carry-over (deferred to later phases)
- #26 (P2) kube source: cancellation through an API-server proxy — deferred, not a requirement (owner, 2026-11-05). kgwctl fixed the same gap in its own dialer and offered its code.
- Phase 7 deferred (D-011): resumes only on a named convergence question from the program. Its roadmap notes (listener state, parser warnings) stay as constraints.
- `origin.ObjectForName` drops certainty (Phase 8 audit P2-e): no consumer; revisit when one appears.
- xDS convergence investigation (P-009), after Phase 8, design only: see roadmap. Program handoff 2026-11-22 (program merged to kgateway-tooling main, `841f508`).
- Ownership (program): `origin` is the only decoder of `merge.*` and `route_source` metadata. Only route-level `merge.*` is read; listener, RouteConfiguration and vhost levels have no provider. Add one only when a consumer asks.

## Notes
- 2026-11-22: owner reordered Phase 8 certainty-first (D-015, via program session). After Phase 8, this session runs the xDS convergence investigation (design only).
- 2026-11-22: architectural review. Phase 7 deferred (D-011), attribution states its certainty (D-012), enterprise corpus removed (D-013). Phase 8 started.
- 2026-11-21: Phase 6 patch `v0.4.3` (sub-phase 6.7, #31): warning for a skipped HCM; narrowing keeps listeners mid-update. E6 cleanup planned as Phase 8 (#32).
- 2026-11-21: Phase 6 patch `v0.4.2` (sub-phase 6.6, #29): narrowing edge cases. #30 open (unmapped policy fields).
- 2026-11-21: Phase 6 patch `v0.4.1` (sub-phase 6.5, #28, D-010): attribution gaps and `RouteExtra`.
- 2026-11-21: Phase 6 closed: `v0.4.0` tagged, #27 closed, notices kgwctl#10 and snellius#63, program file `3b71687`.
- 2026-11-20: owner started Phase 6 as the E11 route view (D-008); intended vs accepted moved to Phase 7.
- 2026-11-05: library renamed to `xdsview` (D-007, `v0.3.0`).
- 2026-11-05: route origins without `--route` decided as option B (D-006): live mode lists HTTPRoutes. Shipped in `v0.2.1`.
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
