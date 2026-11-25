# Decision log

Append-only. Newest at the bottom. Supersede, don't edit.

## D-001 — Adopt the agentic-engineering-sdlc workflow (2026-11-04, phase 4)
**Context:** krp, kgateway-ui and kgwctl are developed in parallel. krp used project-local phase commands and a `docs/plans/` layout; the other two use the agentic-engineering-sdlc skill.
**Decision:** krp uses the skill from Phase 4 on. Docs moved: `docs/plans/krp-roadmap.md` → `docs/roadmap.md`; phase design, plan and `-complete` records → `docs/phases/phase-<N>/` (`design.md`, `implementation-plan.md`); the 2026-03-06 design and plan → `phase-1/initial-design.md`, `initial-implementation-plan.md`; the 2026-03-07 improvement plan → `phase-1/reviews/initial-design-claude.md`; `possible-future-phases.md` and `TODO.md` → the roadmap's Deferred section.
**Alternatives:** Keep the krp commands — rejected: two workflows across one program, and they lack the cross-repo program step.
**Consequences:** Phases 1–3 records are not backfilled to the templates. Phase 4 starts with a recorded base SHA and a cross-model design review.

## D-002 — krp is the Envoy data-plane library (2026-11-04, phase 4)
**Context:** kgateway-ui (Phases 2–5) and kgwctl cover the Gateway API resource graph, policies and status that krp's old Phases 4–7 planned. Both need krp's Envoy view as a library.
**Decision:** Phases 4–6 are library extraction, data-plane accuracy and mapping, and intended vs accepted config. krp owns the Envoy name → Gateway API ref mapping (program P-001). Vendor policy kinds plug in from outside the core (program P-002).
**Alternatives:** Build the Gateway API graph in krp — rejected: duplicates the UI and the CLI.
**Consequences:** The old Gateway API track is in the roadmap's Deferred section. Exported packages are a contract tracked in the program file.

## D-003 — Test corpus: OSS main, enterprise kept separately (2026-11-04, phase 4)
**Context:** Every existing dump came from the enterprise distribution, and four scenarios use enterprise policy APIs. krp is headed into kgateway OSS (program E6).
**Decision:** Re-record all scenarios on OSS kgateway v2.4.7 and v2.5.0 as the main corpus, rewriting the four enterprise scenarios (and adding manifests for 06, which had none) with OSS `TrafficPolicy`/`GatewayExtension`. Keep the existing enterprise dumps and manifests in `testdata/enterprise/` as extra parser coverage (owner's choice).
**Alternatives:** OSS only, removing the enterprise corpus — not chosen; OSS v2.5.0 only — rejected: 2.4 is still supported.
**Consequences:** `testdata/enterprise/` is removed before the move into kgateway. Tests run on both corpora. Superseded by D-013 and D-018: removed in Phase 8.2.

## D-004 — Standard-library core; Kubernetes in `source/kube` (2026-11-04, phase 4)
**Context:** kgateway-ui's core may not import client-go (UI D-016), and kgwctl brings its own `*rest.Config`.
**Decision:** `pkg/envoy/{model,parser,filter,source}` depend only on the standard library and each other, enforced by a test. Port-forwarding lives in `pkg/envoy/source/kube` and takes an optional `*rest.Config`.
**Alternatives:** One `source` package with client-go — rejected: every consumer would pull in client-go.
**Consequences:** Adding a dependency to the core is a decision entry, not a drive-by import.

## D-005 — `-o json` envelope belongs to the CLI (2026-11-04, phase 4)
**Context:** Consumers need a stable JSON shape to test against; the library's contract is its Go types.
**Decision:** `krp dump -o json` prints `{schemaVersion: "v1alpha1", snapshot, warnings}`. The envelope lives in `cmd/krp`; golden files in `testdata/golden/` pin the shape. `schemaVersion` changes only on removals or renames.
**Alternatives:** Envelope type in `pkg/envoy/model` — rejected: library consumers don't need it.
**Consequences:** Golden diffs in a PR are contract changes and get reviewed as such.

## D-006 — The CLI lists HTTPRoutes in live mode (2026-11-05, after phase 5)
**Context:** Most kgateway route names can be read several ways, because HTTPRoute names and namespaces both allow `-`. `origin.Kgateway` resolves them only with `Known` HTTPRoutes or route-source metadata, so `krp dump` showed no route origins without `--route`. The Phase 5 design's non-goal "reading Kubernetes objects" was extrapolated from D-002, not an owner decision.
**Decision:** With `--gateway` or `--deployment`, `krp dump` lists HTTPRoutes (names and namespaces) through its existing Kubernetes connection and passes them as `Known`. The listing lives in `pkg/envoy/source/kube` (client-go is already there); the stdlib-only core is unchanged. If listing fails (for example RBAC), krp warns and continues without it. The README also documents `enableRouteSourceMetadata` (option E).
**Alternatives:**
- Leave as is (A).
- Guess from namespaces seen in the dump (C): rejected, it can be wrong.
- Print all readings (D): rejected, too noisy.
- Documentation only (E): kept as an addition, not a replacement.
**Consequences:** Live mode needs `list httproutes` (cluster-wide) for route origins; without it, output is as before. `--file` is unchanged. This doesn't build the Gateway API graph D-002 assigns to the UI and the CLI: it reads names only.

## D-007 — The library is `xdsview`; the CLI stays `krp` (2026-11-05, after phase 5)
**Context:** Since Phase 4 the repo's main product is a library that kgwctl and kgateway-ui import to fetch, parse and explain Envoy config. "Kgateway Route Printer" names only the CLI.
**Decision:** Rename the repo and Go module to `github.com/DuncanDoyle/xdsview`, tagged from `v0.3.0`. Package paths under `pkg/envoy/...` and the `krp` binary in `cmd/krp` are unchanged.
**Alternatives:** Keep `krp` (rejected: misnames the library); `xdsviewer` (rejected: Go favours short noun module names, and "viewer" reads as the app).
**Consequences:** Consumers import the new path; nothing imported the old one. GitHub redirects old repo URLs. Phase docs keep the old path as history.

## D-008 — Phase 6 is the route view for consumers; intended vs accepted moves to Phase 7 (2026-11-20, phase 6)
**Context:** kgwctl Phase 3 (`proxy routes`, edge E11, #27) needs xdsview's tree, the effective filter chain per route, and listener state. Snellius Phase 12 (E4) reads the same model. The owner agreed this belongs in xdsview, and on 2026-11-20 he started the phase.
**Decision:** Phase 6 delivers E11: `pkg/envoy/chain`, listener state, and the stdlib-only `pkg/envoy/render`, which the `krp` CLI and TUI style. Intended vs accepted becomes Phase 7.
**Alternatives:**
- kgwctl writes its own view (rejected: two views drift).
- The effective chain as a `filter.Filter` option (rejected: filtering narrows a snapshot, the chain is a per-route answer, and Snellius needs it without narrowing).
**Consequences:** `krp`'s text layout follows the shared shape. Golden `.txt` files make the plain text a reviewed contract.

## D-009 — `model.WeightedCluster` holds per-filter config and is no longer comparable (2026-11-21, phase 6)
**Context:** The approved Phase 6 model keeps each backend's raw `typed_per_filter_config` on `WeightedCluster`, so `chain` can mark a chain `Conditional` and Phase 7 can compare the config itself. A map field makes the struct non-comparable in Go. The phase review (#10) noted that the design called library changes additive.
**Decision:** Keep the field on `WeightedCluster`. Treat it as a breaking change for Go callers that compare the struct with `==`, use it as a map key, or pass it to `slices.Equal`. JSON stays additive (`v1alpha1`).
**Alternatives:**
- A separate `Route` map keyed by backend name (rejected: names can repeat in a split, and the data belongs with the backend).
- A one-bit flag (rejected in the design review: Phase 7 needs the config).
**Consequences:** Callers use `reflect.DeepEqual` or compare fields. The `v0.4.0` release notes and the consumer notices name the change.

## D-010 — `render.Options.RouteExtra` in the `v0.4.1` patch (2026-11-21, phase 6)
**Context:** kgwctl adds its own note under some route lines. It finds the line by counting `Style` calls with `ClassRoute`, which depends on render's traversal order.
**Decision:** Add `RouteExtra func(r RouteRef) []string` to `render.Options`. `RouteRef` holds the listener, filter chain, vhost and route indexes. The lines print under the route line, before its filters, like `Extra` does under a filter row. `Style` stays filter-scoped. The owner approved shipping it in `v0.4.1` with the attribution fixes (#28), as an additive API in a stabilization sub-phase.
**Alternatives:**
- Pass a route ref to `Style` for `ClassRoute` (rejected: `Style` styles text; adding lines is `Extra`'s job).
- Ship it alone as `v0.5.0` (owner chose `v0.4.1`).
**Consequences:** Additive. Consumers drop traversal-order assumptions.

## D-011 — Defer intended vs accepted; no generic xDS diff (2026-11-22, phase 7)
**Context:** Phase 7 planned to diff kgateway's `/snapshots/xds` against Envoy's `/config_dump`. An architecture review questioned its operational value. The evidence is in `docs/phases/phase-7/assessment.md`. In a converged system the diff is empty by construction. Every difference is a convergence state that controller metrics and proxy stats already report (Snellius KGWUI-001, 024, 025, 026). A wrong translation that Envoy accepts doesn't show up in the diff. The controller keeps no per-proxy ACK record. Its admin API is kgateway-specific, unauthenticated, and encoded differently from `config_dump`.
**Decision:** Option C. Phase 7 is deferred, not designed. xdsview builds no generic intended-vs-accepted diff. Convergence diagnostics (ACK/NACK, staleness, warming and draining, replica drift, version correlation) are a program-level investigation. xdsview takes a phase only when that investigation names a question that needs Envoy-config evidence, with an operator scenario. Small Envoy-side additions, such as accepted versions per resource or a replica comparison, can then be scoped on their own.
**Alternatives:**
- A, keep Phase 7: rejected for the reasons above.
- B, convergence diagnostics as an xdsview phase: rejected for now. Most inputs are controller metrics and per-pod stats outside this library, which Snellius already reads.
**Consequences:** Phase 8 is next. E4's "intended vs accepted" and kgwctl's deferred `proxy diff` have no provider until the program decides. D-009's per-backend config stays, because `chain` uses it.

## D-012 — Attribution states its certainty; policy semantics stay with the controller (2026-11-22, phase 8)
**Context:** The program agreed that the kgateway controller owns policy semantics: no tool reimplements attachment, precedence, inheritance, merge or override, and heuristics are labelled. An audit of `origin` and `chain` found claims stated as fact:
- policy contributions carry no certainty and render like facts;
- the OSS field table applies to any policy Kind;
- extension contributions drop their origin's `Inferred`;
- text output doesn't separate inferred from exact.

**Decision:**
1. Every contribution carries a certainty. A policy contribution is at most `inferred`. The `merge.*` metadata proves that kgateway merged the field into the route. Which filter config the field produced comes from a table.
2. The OSS field table applies only to kgateway's own policy Kind. Other Kinds attribute only through a plugin.
3. Text output marks `inferred`, as it already marks `ambiguous`.
4. Exact stays for `route_source` identity, and `Route.Policies` stays as metadata-backed.

**Alternatives:**
- Drop policy contributions entirely: rejected, because the metadata is real evidence, and consumers show it.
- Move the most-specific-level rule out of `chain`: rejected, because no requirement needs it. It is labelled `inferred` instead.
**Consequences:** JSON gains `certainty` on contributions (additive). Text output changes, so consumer goldens change. Released as a minor version.

## D-013 — Phase 8 removes the enterprise corpus (supersedes D-003's enterprise part) (2026-11-22, phase 8)
**Context:** D-003 kept the enterprise dumps as extra parser coverage until the move into kgateway (E6). kgwctl's move now waits on it (#32). An inventory showed that the OSS dumps (2.4.7, 2.5.0, 2.5.1) already cover every enterprise-backed test, except a real dump with router upstream filters.
**Decision:** Delete `testdata/enterprise/` and its goldens. Repoint the tests to OSS dumps, and cover upstream filters with a hand-built fixture at the render boundary. Hand-built fixtures use neutral names. The enterprise transformation type leaves the core mapper and the sides table, and a test plugin shows the seam instead. Git history isn't rewritten unless the owner asks.
**Alternatives:**
- Keep the corpus in a separate module: rejected, because nothing outside this repo needs it.
- Rewrite history: owner decision, not assumed.
**Consequences:** Enterprise dumps lose their transformation attribution and show `?` sides for that filter. A plugin can restore the attribution but not the sides; a sides hook waits for a consumer that needs one.

## D-014 — Design-review amendments to D-011 and D-012 (2026-11-22, phase 8)
**Context:** Codex's design review (`docs/phases/phase-8/reviews/design-codex.md`) found three things:
- D-011 overstated what existing signals explain;
- D-012 still let `Exact` route origins carry name-parsed indices;
- D-012 kept a level rule that picks a filter on a translation assumption.

**Decision:**
1. D-011 stands (option C), qualified. Existing signals don't cover per-replica version drift, or a non-LDS rejection whose counter has stopped. Those are candidate questions for the program's convergence investigation, not reasons to diff.
2. A policy field is linked to a filter entry only when exactly one entry on the route matches it, at any config level. Otherwise it stays on `Route.Policies` only. This supersedes D-012's rejected alternative 2. It also reverses the 6.5 rule that gave a route's disable policy to the route-level helper beside an inherited provider.
3. `RouteOrigin` records an index certainty. It is `inferred` when an index came from the route name under `route_source` identity.
4. `PolicyRef` carries a certainty. The built-in reader returns `exact`, and only when the ID's group and kind match the metadata key.

**Alternatives:** keep the level rule, labelled `inferred` (rejected: `inferred` still names one winner, based on how kgateway translates inheritance).
**Consequences:** Fewer policy contributions where several filters match. JSON gains `certainty` on `PolicyRef` and `indexCertainty` on `RouteOrigin`, both additive.

## D-015 — Phase 8 runs certainty first (2026-11-22, phase 8)
**Context:** kgwctl `proxy routes` already ships policy contributions with no certainty (program edge E13). The owner decided, relayed by the program session, that attribution certainty goes first.
**Decision:** Sub-phase order becomes 8.1 certainty and the filter link, 8.2 corpus, 8.3 mapper. Scope is unchanged.
**Alternatives:** the reviewed order (corpus, mapper, certainty): rejected, because it delays the labels kgwctl needs.
**Consequences:** 8.1 regenerates the enterprise goldens that 8.2 then deletes. The design review's findings still apply; only the order changed.

## D-016 — Owner approval of Phase 8; two releases (2026-11-22, phase 8)
**Context:** the design and plan (D-012 to D-015) needed owner approval, with four open questions.
**Decision:** The owner approved the following:
1. The Phase 8 design and plan, in the D-015 order.
2. Dropping the 6.5 level tie-break (D-014 item 2). The `global_disable/*` table entries stay.
3. No git history rewrite. Tags `v0.1.0`–`v0.4.3` are in the Go checksum database, so a rewrite can't remove published content.
4. Two releases: `v0.5.0` after 8.1 (certainty, E13), and `v0.6.0` after 8.3 (vendor-neutral core, E6).

**Alternatives:** one `v0.5.0` at the end (rejected: E13 would wait for unrelated work).
**Consequences:** A phase cross-model review of `f8b4af6..` runs before the `v0.5.0` tag, and again before `v0.6.0`. kgwctl bumps twice; the second bump changes no OSS output. An exact policy-to-filter link needs controller metadata that names the filter; this is proposed to the program as an upstream ask.

## D-017 — Correction: the 6.5 tie-break doesn't affect kgwctl's dump (2026-11-22, phase 8)
**Context:** D-014, the plan and the owner briefing said that kgwctl's `09-kgwctl-policies` dump would lose the `public-no-extauth` link on `global_disable/ext_auth`. 8.1's tests showed this was wrong. The provider `ext_auth/kgwctl-proxy/authz` has per-filter config only on the orders route. On the public route it has no per-filter config and stays off by the HCM default, so it isn't a candidate.
**Decision:** No change to D-014 itself: the tie-break still goes. The claim about dump 09 is corrected. That link stays, labelled `(extAuth, inferred)`.
**Alternatives:** n/a (a factual correction).
**Consequences:** The tie-break decided only the hand-built `TestAnnotate_DisableBesideInheritedProvider` case, which 8.1 replaces with `TestCertainty_OneCandidate`. The goldens show whether any recorded dump drops a contribution.

## D-018 — The enterprise corpus moves downstream (amends D-013) (2026-11-22, phase 8)
**Context:** D-013 deletes the enterprise dumps. The owner decided that a downstream consumer keeps them, in its own private repository. That repository also holds its `origin` plugin behind the `origin.Chain` seam (P-002).
**Decision:** D-013's "delete" becomes "move". For xdsview, 8.2's scope is unchanged: the corpus leaves this repository, and git history stays. The downstream copy is that repository's own work, taken from this history; xdsview pushes nothing there. Where an enterprise-only test shows parser behaviour that no OSS dump covers, xdsview keeps a neutral hand-built fixture (as D-013 does for upstream filters).
**Alternatives:** keep the corpus here (rejected: P-002, E6).
**Consequences:** No change to 8.2's plan or to consumers.

## D-019 — Adopt P-010: control-plane-agnostic core, Phase 9 contract break (2026-11-23, phase 8)
**Context:** The program decided P-010 and edge E15 (program `c304e76`, owner-approved, relayed by the program session). xdsview becomes control-plane agnostic: Envoy semantics in the core, product modules add provenance only.
**Decision:** xdsview adopts P-010 as written in the program's "Control-plane-agnostic xdsview" section. Phase 8 and the P-009 investigation are unchanged. A new Phase 9 follows them: one coordinated contract break per the program's "Phase 9 scope", then core correctness in the same phase. Phase 9 starts with design, plan and cross-model review; kgwctl and Snellius review the design before owner approval (program open question 7).
**Alternatives:** additive changes per consumer request (rejected by the program: several consumers need the same break, and P-010 rule 7 allows one).
**Consequences:**
1. Supersedes the carry-over "add listener, RouteConfiguration and vhost `merge.*` levels only when a consumer asks". E15 scope 5 is that consumer case.
2. Retires the carry-over "sides hook on a consumer's request": modules never supply sides (P-010 rule 3).
3. The kgateway knowledge in `origin` and `filter` moves into a kgateway module. `docs/roadmap.md`'s architecture diagram changes when the package moves.
4. Public hygiene: tests use neutral fixtures only; other control planes are tested downstream.

## D-020 — Phase 9 release and API shape (2026-11-25, phase 9)
**Context:** Phase 9 (D-019) needed two decisions before its design: how to release, and the Go API shape that separates execution from provenance. Codex was consulted without the author's pick (`phases/phase-9/reviews/approach-codex.md`).
**Decision:**
1. One tag at phase close, with the break and the core-correctness fixes (owner, 2026-11-24, option A).
2. Position-keyed claims (owner, 2026-11-25, option 1): `parser.Parse` returns a snapshot with occurrence IDs; `chain.Effective` returns chains keyed by route ID and calls no module; `provenance.Compose(snap, modules...)` returns typed claims in an indexed result; conflicts of equal certainty are ambiguous. Codex agreed with both.

**Alternatives:** B, tag the break first, fixes in a later minor (rejected: two rounds of consumer goldens). A provenance tree mirroring the snapshot (rejected: rules 4–6 still need IDs and per-claim evidence; Snellius walks levels to find one block's dependencies). Keeping `origin.Mapper` with claims (rejected: callbacks lack occurrence and parent context for scope 4–5).
**Consequences:** `origin` and `filter` move into `adapter/kgateway`. IDs are valid in one dump only (P-011 rule 6). kgwctl migrates once; Snellius Phase 12 targets the tag.

## D-021 — Phase 9 runs 9.4 before 9.3 (2026-11-25, phase 9)
**Context:** 9.3 locks the provenance API, and the program asks kgwctl and Snellius to review the design first (kgwctl#14, snellius#70, filed 2026-11-25). 9.4 changes only `model`, `parser` and `render`.
**Decision:** Run 9.4 (core correctness) before 9.3 (owner, 2026-11-25).
**Alternatives:** wait for both reviews before any more work (rejected: idle time, no dependency).
**Consequences:** 9.3 builds on the 9.4 snapshot types. 9.4's goldens change before the `v1alpha2` switch; both land in the one tag.
