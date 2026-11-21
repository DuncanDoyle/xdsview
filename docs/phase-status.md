# Phase status

## Phase
Phase: 6 — Route View for Consumers
Base: 40a5863
Docs: docs/phases/phase-6/
Status: design approved 2026-11-21; 6.1 done; implementing 6.2.

### Queue
- [x] Q1 Phase 6 design + plan (`design.md`, `implementation-plan.md`)
- [x] Q2 Design + plan cross-model review and triage (reviews/design-codex.md)
- [x] Q3 Owner approval of design (2026-11-21; both open questions answered "yes")
- [x] Q4 6.1 Model and parser — 45e1f43, phase-6.1-complete.md
- [ ] Q5 6.2 `pkg/envoy/chain` ← CURRENT
- [ ] Q6 6.3 `pkg/envoy/render` and krp
- [ ] Q7 Phase review, `v0.4.0`, program file, #27, notices

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [ ] design+plan cross-model review + triage: docs/phases/phase-6/reviews/design-codex.md
- [ ] phase cross-model review + triage: docs/phases/phase-6/reviews/phase-codex.md
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-6-complete.md written, roadmap updated, tracked items closed

## Sub-phase
Sub-phase: 6.2 — `pkg/envoy/chain`, attribution and narrowing
Kind: planned
Base: 45e1f43
Brief: docs/phases/phase-6/phase-6.2-brief.md

### Allowed work
Features within the approved design.

### Not allowed
Design changes without a decisions.md entry and owner approval.

### Queue
- [ ] S1 Side table (Envoy v1.39.6 `aacd0340`, cited) and `chain.Effective`: table tests first ← CURRENT
- [ ] S2 Corpus checks on `03` (v2.4.7, v2.5.0) and `00`
- [ ] S3 `origin.PolicyFilters`, Kgateway field table, contributions, `chain.Annotate`
- [ ] S4 `NarrowedByKnown`; narrowing on origins; `filter.Ambiguous`; non-active listeners kept
- [ ] S5 Boundary test; phase-6.2-complete.md; message kgwctl

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`, read at `780ad25`)
Waiting on: nothing (no provider edge needed)
Consumers: E11 (kgwctl Phase 3.2, #27); E4 (Snellius Phase 12, formerly kgateway-ui Phase 9)

## Carry-over (deferred to later phases)
- #26 (P2) kube source: cancellation through an API-server proxy — deferred, not a requirement (owner, 2026-11-05)

## Notes
- 2026-11-21: kgwctl reports (peer FYI, not a request) that its owner required the #26 proxy gap fixed in kgwctl 3.1's own dialer (CONNECT/SOCKS5, conn closed on ctx). It can share the code for #26 once tested.
- 2026-11-20: owner started Phase 6 as the E11 route view (D-008); intended vs accepted moves to Phase 7. Request: #27 from kgwctl.
- 2026-11-05: library renamed to `xdsview` (D-007, `v0.3.0`).
- 2026-11-05: route origins without `--route` decided as option B (D-006): live mode lists HTTPRoutes. Shipped in `v0.2.1`; review in `docs/phases/phase-5/reviews/d006-codex.md`, live kind check on 04 passed (4 of 4 routes inferred).
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
