# Phase status

## Phase
Phase: 4 — Library Extraction
Base: e1c5666890cfc0f05321aabaa537f0f042f350ce
Docs: docs/phases/phase-4/
Status: design + plan approved 2026-11-04; implementing. Phases 1–3 complete (`docs/phases/phase-3/phase-3-complete.md`); roadmap restructured and workflow moved to agentic-engineering-sdlc on 2026-11-04 (D-001, D-002).

### Queue
- [x] Q1 Phase 4 design (`design.md`)
- [x] Q2 Phase 4 implementation plan (`implementation-plan.md`) — planned sub-phases 4.1, 4.2, 4.3
- [x] Q3 Cross-model review of design + plan (7 accepted: reviews/design-codex.md), owner approved 2026-11-04
- [ ] Q4 Sub-phase 4.1 — library move and API ← CURRENT
- [ ] Q5 Sub-phase 4.2 — `-o json` and golden files
- [ ] Q6 Sub-phase 4.3 — OSS corpus on v2.4.7 + v2.5.0, enterprise corpus moved

### Exit gates
- [ ] tests/vet pass (observed)
- [ ] all queue items done or deferred
- [x] design+plan cross-model review + triage: docs/phases/phase-4/reviews/design-codex.md
- [ ] phase cross-model review + triage: docs/phases/phase-4/reviews/phase-codex.md
- [ ] decisions.md / design / plan in sync with code
- [ ] phase-4-complete.md written, roadmap updated, tracked items closed

## Sub-phase
Sub-phase: 4.1 — Library move and API
Kind: planned
Base: 012c61e16ca3a136c1a5532b53274f13d504cbd5
Brief: docs/phases/phase-4/phase-4.1-brief.md

### Allowed work
Features within the approved design (design.md § Package layout, API, Ownership).

### Not allowed
Design changes without a decisions.md entry and owner approval.

### Queue
- [ ] S1 Baseline CLI output from base binary ← CURRENT
- [ ] S2 Move model/parser/filter to pkg/envoy; Parser io.Reader + Result; filter.Options + AllRules
- [ ] S3 pkg/envoy/source (Source, File, Admin)
- [ ] S4 pkg/envoy/source/kube with tunnel cleanup on every path
- [ ] S5 CLI wiring; delete internal/envoy
- [ ] S6 Import-boundary test
- [ ] S7 client-go v0.36.2, go 1.26.0
- [ ] S8 LICENSE + README
- [ ] S9 External consumer check; byte comparison; tests/vet

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md (local: `../kgateway-tooling-claude/program.md`)
Waiting on: none (program file checked 2026-11-04 at `908a199`; no open `cross-repo` issues in krp)
Consumers: E1 (kgwctl Phase 2), E4 (kgateway-ui Phase 9), E6 (move into kgateway) — Phase 4 delivers `v0.1.0`; E5 (mapping) is Phase 5

## Carry-over (deferred to later phases)
- #11 (P2) `prefix_rewrite` route action not captured — Phase 5
- #13 (P3) `weighted_clusters` not captured (traffic splitting) — Phase 5
- #15 Support HTTPRoute filters — Phase 5
- #16 RFE: `--rule <name>` when the HTTPRoute rule name reaches the standard channel — Phase 5, if promoted
- #17 Collect multi-route config dumps for E2E filter tests — Phase 5

## Notes
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
