# Advisory review — Phase 9 and the xdsview repo (Fable)

**Scope:** Phase 9 at 8209553, plus the xdsview repo as a whole
**Advisor:** Fable, via the program session
**Source:** two reports in the program repo (kgateway-tooling), not copied here and not published: the Phase 9 advisory report (F1–F6) and the xdsview advisory report (F1–F5)
**Date:** 2026-11-26
**Cross-model:** not a required cross-model review. It is an advisory read; the Codex reviews in this folder remain the required ones.

Local file links and vendor names redacted; reviewer text otherwise unchanged.

IDs are the program's. A-items come from the Phase 9 report, B, E and A8 from the xdsview report. A4 is tracked in the program.

## Triage

| ID | Report | Finding | Dimension | Decision |
|---|---|---|---|---|
| A1 | Phase 9 F1 | No kgateway block is gap-free, and a gap has no reason | architecture | done: `Index.Inputs` with status, reason, candidates and `Complete` (ad3dcf6, 5887ced, aefc86c) |
| A2 | Phase 9 F2 | `Dependencies` results are behaviour that rule 7 doesn't protect; raw `Metadata` is no input | architecture | done: results outside rule 7 until Snellius Phase 12 is designed; raw metadata decision recorded (f6d5b1f) |
| A3 | Phase 9 F3 | Scoped `merge.*` tested on hand-built fixtures only | correctness | done: kind capture on kgateway v2.5.2, scenario `10-policy-levels`, matches the fixtures |
| A5 | Phase 9 F4 | `Guess` and the route name kind have no consumer | architecture | done: both removed; RDS names need the consumer's Gateway context (25178f4) |
| A6 | Phase 9 F5 | `Module` doc promised a module may change its input | architecture | done: a module must not modify the snapshot; the copy is a safeguard (ba42115) |
| A7 | Phase 9 F6 | design.md is not the contract as built | both | done: design.md rewritten to match the code, with a "Changes since consumer review" list (354d765) |
| B4 | xdsview F1 | Version-keyed tables have no bump trigger and no check against the dumps | both | done: `TestVersionTables_MatchCorpus` ties the sides table, `CheckedMinors` and the policy table to the recordings (69cee3c). Standing roadmap item "Support a new kgateway minor or Envoy release" added |
| A8 | xdsview F2 | Neutral core paths tested only on hand-built dumps | correctness | partly accepted, see below; the owner decides |
| E3 | xdsview F3 | Stdlib rule not enforced on `provenance` and `adapter/kgateway` | architecture | done: `TestContractImportsOnlyStdlib` (98180b5) |
| E4 | xdsview F4 | Assertion kinds are sealed and shaped by kgateway | architecture | deferred to the owner, with A4 |
| E2 | xdsview F5 | Roadmap goal, diagram and deferred sections are out of date | architecture | at Phase 9 close |

**A8.** Some fixtures need input Envoy cannot produce, so they stay hand-built:
- `cluster-selectors`, `matchers` (custom matcher) and `route-actions` (`filter_action`, which Envoy doesn't implement) use `example.*` extension types Envoy doesn't register.
- `coverage` has an unknown config dump section and an unreadable `@type`.
- `ecds` has an entry whose type is not in the filter's `type_urls`.

The rest (`static-bootstrap`, `tcp-proxy`, `default-chain`, `ecds-network`, `upstream-extproc`, and the found and default cases of `ecds`) could be recorded from a standalone Envoy with a static bootstrap. Proposal: record them after the `v0.7.0` tag, on the Envoy versions the corpus runs, and keep the hand-built ones above with this list as their reason. The owner decides whether that waits for the tag.
