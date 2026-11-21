# Phase status

## Phase
Phase: 6 — Route View for Consumers
Base: 40a5863
Docs: docs/phases/phase-6/
Status: design approved 2026-11-21; 6.1 and 6.2 done; 6.3 next.

### Queue
- [x] Q1 Phase 6 design + plan (`design.md`, `implementation-plan.md`)
- [x] Q2 Design + plan cross-model review and triage (reviews/design-codex.md)
- [x] Q3 Owner approval of design (2026-11-21; both open questions answered "yes")
- [x] Q4 6.1 Model and parser — 45e1f43, phase-6.1-complete.md
- [x] Q5 6.2 `pkg/envoy/chain` — 5490926, phase-6.2-complete.md
- [ ] Q6 6.3 `pkg/envoy/render` and krp ← CURRENT
- [ ] Q7 Phase review, `v0.4.0`, program file, #27, notices

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [ ] design+plan cross-model review + triage: docs/phases/phase-6/reviews/design-codex.md
- [ ] phase cross-model review + triage: docs/phases/phase-6/reviews/phase-codex.md
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-6-complete.md written, roadmap updated, tracked items closed

## Sub-phase
Sub-phase: 6.3 — `pkg/envoy/render` and krp
Kind: planned
Base: 371297e
Brief: docs/phases/phase-6/phase-6.3-brief.md

### Allowed work
Features within the approved design.

### Not allowed
Design changes without a decisions.md entry and owner approval.

### Queue
- [ ] S1 `render.Text`, options, goldens, determinism/ANSI/whitespace tests ← CURRENT
- [ ] S2 `internal/renderer` and TUI over `render`; navigation test
- [ ] S3 `krp`: `chain.Annotate` for text and JSON; JSON goldens additive
- [ ] S4 Boundary test with charmbracelet assertion and mutation check
- [ ] S5 Manual TUI run; live kind check (03, ext_authz stats)
- [ ] S6 README; phase-6.3-complete.md; message kgwctl

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
