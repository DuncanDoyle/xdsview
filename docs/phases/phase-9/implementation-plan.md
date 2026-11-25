# Phase 9 — Control-Plane-Agnostic Contract: implementation plan

Design: `design.md` (approved by the owner, 2026-11-25). Four planned sub-phases, each a reviewable change that keeps `go test ./...` and `go vet ./...` green. The tag waits for the last one (D-020).

Rules for every sub-phase:
- Tests first. A test for changed behaviour fails before the change, for the expected reason.
- Test at the highest boundary that shows the behaviour: `cmd/krp` text and JSON goldens for output, the public package API for contract rules.
- Goldens change only through reviewed `-update` runs. Each sub-phase's complete record lists which goldens changed and why.
- Exported doc comments follow Go doc conventions; code comments are 1–2 lines.

Order: IDs first, because every later piece keys on them. Execution next, so the core is pure before provenance moves. Then the contract and the module. Core correctness last, on the new types.

## 9.1 — Snapshot and occurrence IDs (R3, part of R11)

1. Tests first:
   1. **IDs.** Every listener, filter chain, network filter, HCM route config use, virtual host, route, weighted backend and HTTP and upstream filter slot in every scenario dump has a non-empty ID, unique in its snapshot.
   2. **`Within`.** A route ID is within its vhost, HCM, filter chain and listener; not within a sibling. `L1` does not contain `L10/...`.
   3. **Shared route config.** A hand-built dump with two HCMs on one RDS name: one source (`RouteConfigs`), two uses with distinct vhost and route IDs.
   4. **Duplicate RDS names.** Two sources with one name: both listed; the referencing HCM has `RouteConfigState` `ambiguous`, no join, and a warning with its subject. Fails today (the later entry wins).
   5. **Typed warnings.** Each existing warning keeps its text and gains a code and, where it has one, a subject ID.
   6. **Prune.** `model.Prune` keeps the selected routes and their ancestors and every non-active listener; IDs are unchanged. The current `filter` package tests that cover pruning are ported to it.
   7. **Independent parses.** Two parses of one dump are deep-equal, IDs included.
2. Implement in `model` and `parser`. `parser.Parse` returns `*model.Snapshot`; `EnvoySnapshot` is renamed. `origin`, `filter` and `chain.Annotate` are adapted mechanically so krp keeps working; their behaviour doesn't change.

**Done:** tests pass. krp JSON goldens gain IDs, `routeConfigs` and typed warnings only. Text goldens unchanged.

## 9.2 — Pure execution (R1, part of R11)

1. Tests first:
   1. **Boundary.** `boundary_test.go` fails when `model`, `parser`, `chain` or `render` import `provenance`, `adapter/...`, `origin` or `filter`. Fails today (`chain` imports `origin`).
   2. **Effective.** `chain.Effective(snap)` returns one chain per route ID, each entry carrying its slot ID. Run state and sides are `Tri`. Existing chain tests are ported, values unchanged.
   3. **Wrappers.** A Composite sets `Wrapper: composite`; an ExtensionWithMatcher sets `Wrapper: extensionWithMatcher` and `Conditional`. Its sides stay its inner filter's.
   4. **Render by ID.** `render.Text(w, snap, chains, opts)` with `Style` and `Notes` keyed by ID gives today's text for every scenario when krp's notes reproduce today's origin and contribution lines.
   5. **Sides table check (R14).** Every HTTP filter type in the corpus and fixtures is in the sides table or a recorded exclusion. The table names its Envoy version.
2. Implement. Contributions leave `ChainEntry`; krp computes them in its own glue from `origin` until 9.3 replaces it. `chain.Annotate` is removed.

**Done:** tests pass. Text goldens unchanged. JSON goldens change only in chain shape (slot IDs, `Tri`, no contributions inside entries).

## 9.3 — Provenance contract and kgateway module (R2, R4–R10, R13)

1. Tests first:
   1. **Invariance (R2).** For every scenario: parse twice; compose the kgateway module on one. Both snapshots and both `Effective` results are deep-equal. A test module that mutates its input changes neither.
   2. **Compose rules.** A claim with an unknown subject, no evidence, or certainty over its kind's cap is dropped with an index warning. Two modules with distinct `RouteSource` at equal certainty resolve ambiguous with both candidates; at unequal certainty the higher wins and both claims stay listed. Output order is deterministic across runs.
   3. **Claims parity.** Through krp's new path, every scenario's text golden is unchanged except where a deferred P1 fix applies.
   4. **`merge.*` levels (R5).** Records at listener, route config and vhost level become `PolicyRecord` claims on those subjects, level-local. Try a kind capture first (kgateway `v2.5.x`, policies at Gateway and listener level); otherwise hand-built fixtures, and the complete record says so.
   5. **Filter link (rule 4).** One matching slot: a `FilterLink` naming the record claim and the slot, `inferred`. Two: none, the record stays unplaced.
   6. **Deferred P1s.** #2: `--rule 0` selects a route whose rule name is `api.v1`. #4: `name` and `module` disagree: no link. #5: a Composite-wrapped rustformation filter gets its link. Each fails today.
   7. **Dependencies (R6).** A vhost block lists every object its claims rest on; a route with no route claim appears in `Unattributed`.
   8. **Selection (R8).** krp `--route`, `--rule` and live `Known` narrowing give today's routes; ambiguous candidates and narrowings are listed in `Selection`; the index still holds every candidate.
   9. **NameMapper (R9).** Cluster and route config names give certainty, kind and candidates; an ambiguous name lists all.
   10. **Policy-field table check (R14, #30).** Every `merge.*` field in the corpus is in the table or a recorded exclusion. The table names its kgateway version.
   11. **krp `v1alpha2` (R10).** JSON goldens regenerated and reviewed; schema doc updated.
2. Implement `pkg/envoy/provenance` and `pkg/adapter/kgateway`. Remove `origin`, `filter` and the provenance fields from `model`. Port krp, `internal/renderer` and the TUI.

**Done:** tests pass. `go list -deps ./pkg/envoy/model ./pkg/envoy/parser ./pkg/envoy/chain ./pkg/envoy/render` shows no `provenance`, `adapter` or third-party package.

## 9.4 — Core correctness (R12)

Each row of design §4 is one test-first item, through krp text and JSON:
1. Missing RDS: `missing` on a complete dump with an active listener; `unknown` when `RoutesConfigDump` is absent, an entry was skipped, or the listener is warming. Built from scenario 00 with the route config removed.
2. Static listeners and route configs, and an inline HCM `route_config`: a neutral static bootstrap fixture.
3. Non-HCM chains: a `tcp_proxy` chain is listed.
4. ECDS: with `EcdsConfigDump`, the config is joined; without it, `Runs` unknown and `State` unresolved, and the filter is not in the request order as running.
5. Matchers: every header and query form, `invert_match`, `ignore_case`, `case_sensitive`; an unmodelled predicate is in `Unparsed`.
6. Default filter chain: parsed with ID `L<n>/Cd`.
7. Cluster selectors: `cluster_header` and a cluster specifier plugin are shown, with no cluster name.
8. Actions: redirect and direct response, body size only.
9. Coverage: every section in a dump is listed as parsed, partial or ignored; a secret section is never decoded; `scoped_routes` gives `unknown`.

Fixtures are neutral and hand-built where no OSS capture shows the case (P-010).

**Done:** tests pass. Every changed golden is listed in `phase-9.4-complete.md` with its reason.

## Close

1. Benchmark `parser.Parse`, `chain.Effective` and `Compose` on a synthetic dump of 5,000 routes; record time and memory (design risk: copy per module).
2. Phase cross-model review of `<base>..HEAD` (Codex), linking program.md "Architecture guidelines".
3. Owner: approve the tag `v0.7.0`, then the program file update (E15) and `cross-repo` notices to kgwctl and Snellius.

## Before 9.3 locks the API

kgwctl and Snellius review the design (program open question 7). The requests are drafted in `.agents-tmp/p9/` and filed only after the owner says yes. 9.1 and 9.2 change no consumer-facing names beyond the snapshot rename, so they can run meanwhile.

## Order and size

| Sub-phase | Packages | Estimate |
|---|---|---|
| 9.1 | model, parser, small glue | ~400 lines + tests |
| 9.2 | chain, render, krp glue | ~350 lines + tests |
| 9.3 | provenance, adapter/kgateway, krp, TUI; removes origin, filter | ~1,200 lines + tests |
| 9.4 | parser, model, render | ~600 lines + tests |
