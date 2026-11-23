# Phase status

## Phase
Phase: next — xDS convergence investigation (P-009), design only; not started. Phase 9 (P-010) follows.
Base: n/a (recorded at start)
Author: n/a
Advisory: none
Docs: docs/investigations/ (to be created)
Status: Phase 8 closed 2026-11-23 (docs/phases/phase-8/phase-8-complete.md). Release handoff below still open.

### Queue
- [x] Q1 tag `v0.6.0` at 10512f6 (owner approved 2026-11-23)
- [ ] Q2 kgwctl `cross-repo` notice for E6 / `v0.6.0` (ask before filing) ← CURRENT
- [ ] Q3 E6 row update through the program session
- [ ] Q4 start the P-009 investigation (ask first)

### Exit gates
n/a (no phase open)

## Sub-phase
none

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`, updated at `c304e76`)
Waiting on: nothing.
Consumers: E6 (kgwctl move into kgateway): `v0.6.0` released. E13 available (`v0.5.0`). E11 available. E15 planned (Phase 9).

## Carry-over (deferred to later phases)
- Phase 8 fix review 4 #2, #4, #5 (P1, owner-deferred to Phase 9 design): dotted rule names lose `--rule <n>`; `dynamic_module_config.module` overrides the library check; Composite-wrapped rustformation loses its link. See phase-8-complete.md.
- #26 (P2) kube source: cancellation through an API-server proxy — deferred, not a requirement (owner, 2026-11-05). kgwctl fixed the same gap in its own dialer and offered its code.
- Phase 7 deferred (D-011): resumes only on a named convergence question from the program. Its roadmap notes (listener state, parser warnings) stay as constraints.
- `origin.ObjectForName` drops certainty (Phase 8 audit P2-e): no consumer; revisit when one appears.
- xDS convergence investigation (P-009), after Phase 8, design only: see roadmap. Keep resource `version_info` and subscription `version_text` distinct before defining any replica comparison (phase review #6). Program handoff 2026-11-22 (program merged to kgateway-tooling main, `841f508`).
- Ownership (program): `origin` is the only decoder of `merge.*` and `route_source` metadata. Only route-level `merge.*` is read today. The other levels come in Phase 9 (P-010, D-019).
- Phase 9 (P-010, E15), after P-009: one contract break, design reviewed by kgwctl and Snellius before approval. File `cross-repo` review requests in both when the design is ready (ask first).

## Notes
- 2026-11-23: Phase 8 closed: phase review and four fix rounds (Codex); three P1s deferred to Phase 9 by the owner. OSS output identical to `v0.5.0`.
- 2026-11-23: program adopted P-010 (`c304e76`): Phase 9 contract break after P-009 (D-019). Phase 8 unchanged.
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
