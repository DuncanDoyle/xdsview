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
   6. **Prune.** `model.Prune` keeps the selected routes and their ancestors, every non-active listener, and every active listener that is warming a replacement, draining or has an error; IDs are unchanged. The current `filter` package tests that cover pruning are ported to it.
   7. **Independent parses.** Two parses of one dump are deep-equal, IDs included.
   8. **Sources and uses.** Scenario 00's `main_route` (inline and in `static_route_configs`) is one inline source that records the static ID; a static entry with no inline match is a source with no use; an RDS name never resolves to a static source.
   9. **Capture.** `parser.Options` defaults to `CaptureUnverified`; `source.File` reports unverified, `source.Admin` and `kube` report `CaptureComplete` through `Source.Capture()`.
2. Implement in `model`, `parser` and `source`. `parser.Parse` returns `*model.Snapshot`; `EnvoySnapshot` is renamed. `origin`, `filter` and `chain.Annotate` are adapted mechanically so krp keeps working; their behaviour doesn't change.

**Done:** tests pass. krp JSON goldens gain IDs, `routeConfigs`, capture and typed warnings. Text goldens unchanged except where a duplicate RDS name now reads `ambiguous`; each such change is listed.

## 9.2 — Pure execution (R1, part of R11)

1. Tests first:
   1. **Boundary.** `boundary_test.go` fails when `model`, `parser`, `chain` or `render` import `provenance`, `adapter/...`, `origin` or `filter`. Fails today (`chain` imports `origin`).
   2. **Effective.** `chain.Effective(snap)` returns one chain per route ID, each entry carrying its slot ID. Run state and sides are `Tri`. Existing chain tests are ported, values unchanged.
   3. **Wrappers.** A Composite sets `Wrapper: composite`; an ExtensionWithMatcher sets `Wrapper: extensionWithMatcher` and `Conditional`. Its sides stay its inner filter's.
   4. **Render by row.** `render.Text(w, snap, chains, opts)` with `Style` and `Notes` keyed by `Row` gives today's text for every scenario when krp's notes reproduce today's origin and contribution lines. One slot under two routes with different config gets different notes per row.
   5. **Sides table check (R14).** Every HTTP filter type in the corpus and fixtures is in the sides table or a recorded exclusion. The table names its Envoy version.
2. Implement. Contributions leave `ChainEntry`; krp computes them in its own glue from `origin` until 9.3 replaces it. `chain.Annotate` is removed. In the same step, the attribution tests move onto that glue at the krp boundary, and `internal/renderer` and the TUI switch to `Row`.

**Done:** tests pass. Text goldens unchanged. JSON goldens change only in chain shape (slot IDs, `Tri`, no contributions inside entries).

## 9.3 — Provenance contract and kgateway module (R2, R4–R10, R13)

1. Tests first:
   1. **Invariance (R2).** For every scenario: parse twice; compose the kgateway module plus a second test module that mutates nested maps, slices and pointers. Both snapshots and both `Effective` results are deep-equal. The test module lives in `provenance_test`, so it uses exported API only (R15).
   2. **Compose rules.** A claim with an unknown subject, no evidence, or certainty over its kind's cap is dropped with an index warning. Duplicate module names fail. Two modules with distinct `RouteSource` at equal certainty resolve ambiguous with both candidates; at unequal certainty the higher wins and both claims stay listed. Two `PolicyRecord`s on one route are both kept. A `FilterLink` with a dangling or rejected record ref, a slot on another HCM, or no per-filter config for the route is dropped. Output order is deterministic across runs.
   3. **Claims parity.** Through krp's new path, every scenario's text golden is unchanged except where a deferred P1 fix applies.
   4. **`merge.*` levels (R5).** Records at listener, route config and vhost level become `PolicyRecord` claims on those subjects, level-local. Try a kind capture first (kgateway `v2.5.x`, policies at Gateway and listener level); otherwise hand-built fixtures, and the complete record says so.
   5. **Filter link (rule 4).** One matching slot: a `FilterLink` naming the record claim and the slot, `inferred`. Two: none, the record stays unplaced. A link to a record on an unrelated listener is dropped. A claim naming an object no evidence names is dropped.
   6. **Deferred P1s.** #2: `--rule 0` selects a route whose rule name is `api.v1`. #4: `name` and `module` disagree: no link. #5: a Composite-wrapped rustformation filter gets its link. Each fails today.
   7. **Dependencies (R6).** A listener block includes the claims of a route config source it shares with another listener. Policy metadata on a source with no use, and on one with two uses, appears in each use's dependencies (none for the unused one). A route block lists the objects of its own claims, its vhost's and route config source's policy records, its listener's claims and its HCM's slot claims. A route with only a policy claim has a `routeSource` gap; a slot with per-filter config and no extension claim has a gap.
   8. **Selection (R8).** krp `--route`, `--rule` and live `Known` narrowing give today's routes; ambiguous candidates, unresolved marker routes and narrowings are listed in `Selection`; the index still holds every candidate. A route whose readings are all outside `Known` stays unresolved. krp live mode without `--route` shows narrowed readings via `Narrow`, as today.
   9. **NameMapper (R9).** Cluster, route config and listener names give certainty, kind and candidates; an ambiguous name lists all. `Guess` on a name that reads as two kinds is ambiguous.
   10. **Policy-field table check (R14, #30).** Every `merge.*` field in the corpus is in the table or a recorded exclusion. The table names its kgateway version.
   11. **krp `v1alpha2` (R10).** JSON goldens regenerated and reviewed; schema doc updated. With `--route`, no field outside `selection` refers to an ID absent from `snapshot`; `selection` entries carry route and vhost names.
   12. **Consumer payload.** An example test builds a kgwctl-shaped route payload (route ID, match, backend, route source, chain with links) from exported API only. This is xdsview's acceptance for E15; kgwctl's own goldens are kgwctl's.
2. Implement `pkg/envoy/provenance` and `pkg/adapter/kgateway`. Remove `origin`, `filter` and the provenance fields from `model`. Port krp, `internal/renderer` and the TUI.

**Done:** tests pass. `go list -deps ./pkg/envoy/model ./pkg/envoy/parser ./pkg/envoy/chain ./pkg/envoy/render` shows no `provenance`, `adapter` or third-party package.

## 9.4 — Core correctness (R12)

Each row of design §4 is one test-first item, through krp text and JSON:
1. Missing RDS: `missing` on a `CaptureComplete` dump with an active listener; `unknown` when the capture is unverified (a file, or a filtered dump), `RoutesConfigDump` is absent, an entry was skipped, or the listener is warming or draining. Built from scenario 00 with the route config removed.
2. Static listeners and route configs, and an inline HCM `route_config`: a neutral static bootstrap fixture.
3. Non-HCM chains: a `tcp_proxy` chain is listed.
4. ECDS: an entry with matching name and type is joined; a type outside `type_urls` is not. No entry plus `default_config` on an active listener: `ecdsDefault`, with and without `apply_default_config_without_warming`. No entry and no default: `Runs` unknown, `State` unresolved, shown in position with a label. A Composite and an ExtensionWithMatcher are `Conditional`.
5. Matchers: every top-level header form and every `string_match` form, query forms, `invert_match`, `ignore_case`, `case_sensitive`, `connect_matcher`; an unmodelled predicate is in `Unparsed` in text and JSON.
6. Default filter chain: parsed with ID `L<n>/Cd`.
7. Cluster selectors: `cluster_header`, a cluster specifier plugin and an inline one are shown, with no cluster name.
8. Actions: redirect and direct response, body size only.
9. Coverage: every section in a dump is listed as parsed, partial or ignored; a secret section is never decoded; `scoped_routes` gives `unknown`.

Fixtures are neutral and hand-built where no OSS capture shows the case (P-010).

**Done:** tests pass. Every changed golden is listed in `phase-9.4-complete.md` with its reason.

## Close

1. Benchmark `parser.Parse`, `chain.Effective` and `Compose` on a synthetic dump of 5,000 routes; record time and memory (design risk: copy per module).
2. Phase cross-model review of `<base>..HEAD` (Codex), linking program.md "Architecture guidelines".
3. Owner: approve the tag `v0.7.0`, then the program file update (E15) and `cross-repo` notices to kgwctl and Snellius.

## Before 9.3 locks the API

kgwctl and Snellius review the design (program open question 7, D-019). The program puts that review before owner approval; the owner approved first (2026-11-25). Open for the owner: file the requests now, and accept that 9.1 and 9.2 run on a local branch meanwhile (design review #10). The requests are drafted in `.agents-tmp/p9/` and filed only after the owner says yes. 9.1 and 9.2 change no consumer-facing names beyond the snapshot rename; nothing merges to `main` before the consumer review.

## Order and size

| Sub-phase | Packages | Estimate |
|---|---|---|
| 9.1 | model, parser, small glue | ~400 lines + tests |
| 9.2 | chain, render, krp glue | ~350 lines + tests |
| 9.3 | provenance, adapter/kgateway, krp, TUI; removes origin, filter | ~1,200 lines + tests |
| 9.4 | parser, model, render | ~600 lines + tests |

## E15 acceptance

xdsview's side is the example payload test (9.3 test 12) and the `v1alpha2` goldens. kgwctl's side is its own gate, tracked in its migration issue: its `ProxyRoutes` payload, the shared `kgwctl.kgateway.dev/v1alpha2` envelope across every command, and reviewed text, JSON and YAML goldens. The program marks E15 done only when both hold.
