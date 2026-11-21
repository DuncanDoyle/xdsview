# Phase status

## Phase
Phase: 6 — Route View for Consumers
Base: 40a5863
Docs: docs/phases/phase-6/
Status: 6.1–6.3 done 2026-11-21; phase review next.

### Queue
- [x] Q1 Phase 6 design + plan (`design.md`, `implementation-plan.md`)
- [x] Q2 Design + plan cross-model review and triage (reviews/design-codex.md)
- [x] Q3 Owner approval of design (2026-11-21; both open questions answered "yes")
- [x] Q4 6.1 Model and parser — 45e1f43, phase-6.1-complete.md
- [x] Q5 6.2 `pkg/envoy/chain` — 5490926, phase-6.2-complete.md
- [x] Q6 6.3 `pkg/envoy/render` and krp — 6713dd7, phase-6.3-complete.md
- [x] Q7 Phase review — reviews/phase-codex.md, triaged: 13 accepted, 1 deferred to Phase 7
- [ ] Q8 (P1) Composite sides from the installed type — src: reviews/phase-codex.md#1 ← CURRENT (proposed 6.4)
- [ ] Q9 (P1) Policy attribution only when one entry matches — #2
- [ ] Q10 (P1) Conditional heading always qualified, response too — #3
- [ ] Q11 (P1) FilterIdx carried, not looked up by name — #4
- [ ] Q12 (P1) Escape control bytes, trim trailing whitespace — #5
- [ ] Q13 (P1) `krp -i` prints the tree when nothing is selectable — #6
- [ ] Q14 (P1) Exact `FilterConfig` type; null `config` is unset — #7
- [ ] Q15 (P2) Wording `(chosen from known HTTPRoutes)`; tell kgwctl — #8
- [ ] Q16 (P2) IPv6 brackets in text; padded number column — #9
- [ ] Q17 (P2) D-009: `WeightedCluster` not comparable — #10
- [ ] Q18 (P3) Navigation test on `View()` — #11
- [ ] Q19 (P3) Boundary test allows only `core` — #12
- [ ] Q20 (P4) Deep-copy per-filter config in the clone — #14
- [ ] Q21 `v0.4.0`, program file, #27, notices

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
