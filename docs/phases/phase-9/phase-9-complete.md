# Phase 9 — Control-Plane-Agnostic Contract (Complete)

**Completed:** 2026-11-26
**Range:** 40822d0..cf7e1d5, plus this record (released as `v0.7.0`)

## Summary
xdsview's core now reads any Envoy config dump without knowing the control plane. kgateway knowledge lives in one module, composed explicitly. This is the one contract break P-010 allows (E15).

What a consumer gets:
1. `parser.Parse` returns a read-only snapshot. Every listener, chain, filter, route config, vhost and route has a snapshot-local occurrence ID.
2. `chain.Effective` computes the runtime filter chain per route. It never calls a module.
3. `provenance.Compose(snap, modules...)` returns an `Index` of claims. Each claim has its own certainty, evidence and producing module. Conflicts stay visible as several candidates.
4. `Index.Dependencies` and `Index.Inputs` list, per block, every input a viewer's access depends on. A missing input is a gap with a reason; the list fails closed.
5. `kgateway.New()` is the kgateway module. `kgateway.Select` picks routes for a query; consumers narrow and prune themselves.

Core correctness came with the break: static listeners and route configs, non-HCM chains, ECDS and wrapper state, the remaining route matchers and actions, default filter chains, missing RDS, and a coverage line. Text and JSON goldens changed and were reviewed. krp JSON moved to `v1alpha2`.

## Sub-phases (phase record only)
| Sub-phase | Kind | Issues | Description |
|---|---|---|---|
| 9.1 | planned | E15 | Snapshot, occurrence IDs, typed warnings, `Capture`, `Prune` |
| 9.2 | planned | E15 | Pure execution: `chain.Effective` keyed by route ID, `Tri`, slots, wrappers |
| 9.4 | planned | E15 | Core correctness (ran before 9.3, D-021) |
| 9.3 | planned | E15, Phase 8 P1s #2 #4 #5 | Provenance contract, kgateway module, krp port, `origin` and `filter` deleted |
| review fixes | stabilization | phase review, rounds r2–r7 | Fail-closed `Dependencies` with an independent oracle test; `Inputs` with gap reasons |
| close fixes | stabilization | advisory, consumer re-reviews | Advisory A1–A7, B4, E3; design rewritten as built; consumer findings |

## Reviews
All files are in `reviews/` and carry a triage table.
- `approach-codex.md`, `design-codex.md`, `design-r2-codex.md`: design and plan. All accepted except one rejected (r2 #11, Snellius code).
- `consumer-kgwctl.md`, `consumer-snellius.md`: consumer design reviews (kgwctl#14, snellius#70). All accepted. Both consumers confirmed the as-built text.
- `9.1-9.2-codex.md`, `9.4-codex.md`: sub-phase reviews. All accepted.
- `phase-codex.md`, `phase-r2-codex.md` to `phase-r7-codex.md`: phase review and six rounds. All accepted. The kind capture (r8) was deferred to the owner, then done (scenario 10).
- `design-as-built-codex.md`: the rewritten design against the code. All accepted.
- `phase9-advisory-fable.md`: advisory, not a required review. A1–A3, A5–A7, B4, E3 done; A8 partly accepted; E4 with A4 to the owner; E2 done at this close.
- `visibility-strategies-codex.md`: consult on route visibility, input to Snellius.

## Waivers
none

## Deliverables
- Packages:
  - core (stdlib only): `pkg/envoy/{model,parser,chain,render,source}`;
  - contract: `pkg/envoy/provenance`;
  - module: `pkg/adapter/kgateway`.
- Deleted: `pkg/envoy/origin`, `pkg/envoy/filter`, the model's provenance fields.
- Tests: `oracle_test.go` and `failclosed_test.go` (fail-closed enumeration), `TestContractImportsOnlyStdlib`, `TestVersionTables_MatchCorpus`, `hygiene_test.go`.
- Scenario `10-policy-levels` (kind capture, kgateway v2.5.2, route-source metadata on) and nine hand-built fixtures.
- Decisions D-020 to D-022. No new dependencies.
- Benchmark at 5,000 routes: Parse 47 ms / 64 MB, Effective 2.5 ms, Compose 39 ms / 85 MB (phase-9.3-complete.md).

## Notes for later phases
- `Dependencies` and `Inputs` results are outside rule 7 until Snellius Phase 12 is designed. Changing them in a minor release is allowed; say so in the notice.
- Raw `Metadata` is not an attribution input. A consumer that shows it needs its own check.
- Without `route_source` metadata, kgateway route names often read several ways. Those routes carry an `ambiguous` routeSource gap. Turning the metadata on in kgateway removes it (program U2, kgateway#15067).
- Route visibility is policy and belongs to Snellius (D-085). The strategies draft went to Snellius and the program session.
- RDS route config names need the consumer's Gateway context; `kgateway.Names()` doesn't map them.

## Deferred / open
- A4 (cross-module filter links) and E4 (sealed assertion kinds): owner decides, tracked in the program.
- A8: record the neutral fixtures Envoy can produce from a standalone Envoy; owner decides.
- kgateway#15067: a comment with the measured metadata cost and the stale-collision case; owner decides.
- #26, #30: carried over.
- Phase 7 stays deferred (D-011).
