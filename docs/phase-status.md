# Phase status

## Phase
Phase: 6 — Intended vs Accepted Configuration
Base: n/a (recorded at phase start)
Docs: docs/phases/phase-6/
Status: not started. Phase 5 complete 2026-11-05, tag `v0.2.0` (`docs/phases/phase-5/phase-5-complete.md`).

### Queue
- [ ] Q1 Phase 6 design (`design.md`) ← CURRENT

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [ ] design+plan cross-model review + triage: docs/phases/phase-6/reviews/design-codex.md
- [ ] phase cross-model review + triage: docs/phases/phase-6/reviews/phase-codex.md
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-6-complete.md written, roadmap updated, tracked items closed

## Sub-phase
none

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`)
Waiting on: to be checked at phase start
Consumers: E4 (kgateway-ui Phase 9, second half)

## Carry-over (deferred to later phases)
- #26 (P2) kube source: cancellation through an API-server proxy — deferred, not a requirement (owner, 2026-11-05)

## Notes
- 2026-11-05: route origins without `--route` decided as option B (D-006): live mode lists HTTPRoutes. Shipped in `v0.2.1`; review in `docs/phases/phase-5/reviews/d006-codex.md`, live kind check on 04 passed (4 of 4 routes inferred).
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
