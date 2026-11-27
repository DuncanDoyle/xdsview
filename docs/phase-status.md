# Phase status

## Phase
Phase: none active. Phase 9 closed 2026-11-26 (`v0.7.0`); record in `phases/phase-9/phase-9-complete.md`. The next phase waits on the owner.

## Sub-phase
n/a

## Program
Program file: https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md
Waiting on: nothing.
Consumers: E15 (this release): kgwctl `proxy routes`, kgwctl Phase 6 via E7, Snellius Phase 12. E6, E11, E13 available.

## Carry-over (for later phases)
- A4 (cross-module filter links) and E4 (sealed assertion kinds): owner decides, tracked in the program.
- A8: record the neutral fixtures a standalone Envoy can produce; owner decides.
- kgateway#15067 comment (metadata cost, stale-collision case): owner decides.
- #26 (P2) kube source: cancellation through an API-server proxy, not a requirement (owner, 2026-11-05).
- #30: the remaining kgateway policy fields.
- Phase 7 deferred (D-011): resumes only on a named convergence question from the program.
- From kgwctl (2026-11-27), owner decides, proposed as one patch (`v0.7.1`) with a Codex review:
  1. (P3 architecture) `Notes` labels ancestor records "inherited policies"; they only show where the controller recorded a policy. kgwctl relabels by matching that exact string. Proposal: wording "policies recorded at", label as an exported constant, tell kgwctl before the push.
  2. (P2 correctness) `combine` (`provenance/index.go`) takes `max(IndexCertainty)`; unset (0) is below Inferred, so an exact reading merged with an agreeing inferred one prints "(partly from name)". Reachable only with a wrapping module (D-023). Proposal: unset in either stays unset, else max; regression test first.

## Notes
- 2026-11-27: D-023 (wrapping module) merged to `main` and pushed (`7094201`), no tag; Snellius told. Two kgwctl notes added to the carry-over, waiting on the owner. Program E15 commit waits in the program session.
- 2026-11-26: Phase 9 closed. Owner said go: merged to `main`, tagged `v0.7.0`. Strategies draft sent to the Snellius and program sessions (owner read it).
- 2026-11-26: owner decisions 1–4 done: bootstrap listeners need no attribution (`a503fc4`, scope fixed `ae0955d`); kind capture with kgateway v2.5.2 matched the fixtures, added as scenario 10 (`86357d8`..`77215bd`); `Index.Inputs` with gap reasons, `Complete` and `Exhaustive` (`ad3dcf6`, `5887ced`, `aefc86c`); Codex rounds r6 and r7 triaged and fixed. Decision 6: both upstream asks are already U2 (kgateway#15067, #15068); the new cost evidence and stale-collision case await the owner for a comment. Decision 5: strategies draft (local, not published), Codex-reviewed, not yet sent. Next, owner: send the draft; merge, push, tag `v0.7.0`; phase record; program E15; notices.
- 2026-11-26: owner discussion on route attribution and Snellius visibility. Agreed direction, not yet built: visibility becomes a pluggable strategy in Snellius (owner idea); xdsview supplies evidence only. Codex consult `reviews/visibility-strategies-codex.md`: feasible; xdsview adds, before the tag, a per-block route-input list (candidates, evidence, specific gap reasons, independent of cluster existence); Snellius owns strategies (`require-route-metadata`, `require-all-route-candidates`, `trust-current-route-inventory` needs a D-085 exception, `require-dump-access` as a separate operator view). Owner view: the stale-collision leak is rare and time-bound, so prefer proportionate rules over fail-closed for it; a coarse "who may see the dump at all" gate comes first. Open for the owner: (1) bootstrap listeners as "nothing to attribute"; (2) kind capture for listener/vhost `merge.*`; (3) a last Codex pass; (4) build the route-input list; (5) send the strategy proposal to the Snellius and program sessions; (6) upstream asks: `EnableRouteSourceMetadata` default-on (measured about 155 bytes per route, 0.1% of a real 1.2 MB dump, 7% of its route section), unambiguous route names behind a setting. Then merge, push, tag `v0.7.0`, `phase-9-complete.md`, program E15, notices. Scratch: a numbers-only size check script (local, not published).
- 2026-11-25 (night): 9.3 done; phase review plus five fix rounds. `Dependencies` is now a fail-closed enumeration with an independent oracle test (`oracle_test.go`). Corpus: no route block is gap-free (kgateway listeners: `gatewaySource` closed by consumer context, `routeSource` ambiguous without `route_source` metadata; static bootstrap blocks always gapped). Owner, pending: (1) kind capture for listener/vhost `merge.*` (R5) or accept fixtures; (2) accept that static bootstrap blocks and non-HCM network filters always gap; (3) a last Codex pass or stop; (4) merge to main, push, tag `v0.7.0`; (5) program file update (E15) and notices to kgwctl and Snellius. Close: `phase-9-complete.md` still to write.
- 2026-11-25 (night): 9.3 done. Part B ported krp to `Compose`/`Select`/`Narrow`/`Prune` with D-022, moved `-o json` to `v1alpha2`, and deleted `origin`, `filter` and the model provenance fields. Parity with the old path held on 671 outputs before the oracle was deleted. Text goldens unchanged; JSON goldens are the new shape. Open for the phase review: part A item 6 (regex matchers, `Route.Cluster`), the `Compose` benchmark. Carry-over items on Phase 8 P1s, `origin.ObjectForName` and `origin` ownership are now handled by the kgateway module and `NameMapper`.
- 2026-11-25 (late): 9.4 done and reviewed (Codex, 9 fixes). Consumer reviews done; API lockable. Next: 9.3. 9.3 must know: `Prune` keeps reported static sources; `RouteConfigUse.Chain` moves with `Route.Chain`; a static listener's Status still says active; cluster upstream ECDS not read (recorded risk); Snellius gaps (policyRecord, dropped, gatewaySource) and kgwctl selection rules are in design §5–§7. All on local branch `duncandoyle/phase9`; nothing pushed beyond `8d381f6`.
- 2026-11-25: 9.4 done on the local branch: static listeners, network filter IDs, default chain, missing RDS, ECDS, matchers, route actions, coverage. Every corpus text and JSON golden changed (bootstrap listeners, coverage line); nine deviations in phase-9.4-complete.md. Not yet cross-model reviewed.
- 2026-11-25: 9.1 and 9.2 done on local branch `duncandoyle/phase9` (not pushed); Codex review of both triaged, four fixes in. Owner, pending: file the kgwctl and Snellius design-review requests (local drafts, not published; link needs `main` pushed); run 9.4 before 9.3?
- 2026-11-25: 9.2 done: `chain.Effective` keyed by route ID, `Tri`, slot IDs, wrappers; `render` hooks keyed by `Row`; contributions computed in krp. `chain` and `render` no longer import `origin`. Text goldens unchanged; JSON chain shape changed, contributions moved top-level. Four deviations in phase-9.2-complete.md.
- 2026-11-25: 9.1 done: IDs, sources and uses, typed warnings, `Capture`, `model.Prune`. Text goldens unchanged; JSON goldens additive. Six deviations listed in phase-9.1-complete.md (scenario 00 `main_route` waits for 9.4 static listeners).
- 2026-11-25: program adopted P-011 and P-012 ("Architecture guidelines", kgateway-tooling `dbf7140`). Review prompts link that section from now on (protocol step 7).
- 2026-11-25: owner chose API option 1 (D-020). Design draft `phases/phase-9/design.md` written; waiting on owner review before the plan.
- 2026-11-24: Phase 9 started at 40822d0 (owner: yes).
- 2026-11-24: design in progress (brainstorming). Owner decided: one tag at the end of the phase, break plus core correctness (option A). Open, asked and unanswered: API shape. Proposed option 1 (position-keyed claims: `parser.Parse` → read-only snapshot with occurrence IDs; `chain.Effective` keyed by ID; `provenance.Compose(snap, modules...)` returns flat claims; conflicts → ambiguous). Rejected: a parallel provenance tree (not additive), keeping `Mapper` (misses scope 4–5). Resume there.
- 2026-11-24: P-009 done (`263055d`). Owner: missing-RDS finding into Phase 9; kgwctl#13, snellius#68 filed; program `0b71089`; M-3 not approved.
- 2026-11-23: Phase 8 closed: phase review and four fix rounds (Codex); three P1s deferred to Phase 9 by the owner. OSS output identical to `v0.5.0`.
- 2026-11-23: program adopted P-010 (`c304e76`): Phase 9 contract break after P-009 (D-019). Phase 8 unchanged.
- 2026-11-22: owner reordered Phase 8 certainty-first (D-015, via program session). After Phase 8, this session runs the xDS convergence investigation (design only).
- 2026-11-22: architectural review. Phase 7 deferred (D-011), attribution states its certainty (D-012), downstream corpus removed (D-013). Phase 8 started.
- 2026-11-21: Phase 6 patch `v0.4.3` (sub-phase 6.7, #31): warning for a skipped HCM; narrowing keeps listeners mid-update. E6 cleanup planned as Phase 8 (#32).
- 2026-11-21: Phase 6 patch `v0.4.2` (sub-phase 6.6, #29): narrowing edge cases. #30 open (unmapped policy fields).
- 2026-11-21: Phase 6 patch `v0.4.1` (sub-phase 6.5, #28, D-010): attribution gaps and `RouteExtra`.
- 2026-11-21: Phase 6 closed: `v0.4.0` tagged, #27 closed, notices kgwctl#10 and snellius#63, program file `3b71687`.
- 2026-11-20: owner started Phase 6 as the E11 route view (D-008); intended vs accepted moved to Phase 7.
- 2026-11-05: library renamed to `xdsview` (D-007, `v0.3.0`).
- 2026-11-05: route origins without `--route` decided as option B (D-006): live mode lists HTTPRoutes. Shipped in `v0.2.1`.
- Phases 1–3 predate the workflow: no base SHAs, no cross-model review files, and no `-complete` records for 1.1 and 3.1–3.4 (summarised in `phase-3-complete.md`). Not backfilled.
- Phase records keep their original `docs/plans/...` paths in the text; D-001 maps them to the new layout.
