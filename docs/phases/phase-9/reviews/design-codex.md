# Design + plan review — Phase 9 (Codex)

Local file links and vendor names redacted; reviewer text otherwise unchanged.

**Scope:** design + plan, `design.md` and `implementation-plan.md` at `ac3cb72`
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Prompt:** design review prompt (local capture, not published); links program.md "Architecture guidelines"
**Date:** 2026-11-25

Phase 9 needs design changes before the API is locked. The main blockers are Snellius’s evidence boundary, missing-RDS detection on offline dumps, and the distinction between an installed filter slot and its per-route execution.

### P1

1. **Architecture · Snellius can treat incomplete evidence as complete.** `Dependencies` checks only descendant claims and flags only routes and slots with no claim above unknown. A route block can therefore omit an inherited vhost or RouteConfiguration policy, a backend with unknown attribution, and slot evidence on the HCM. A policy claim on the route can also mask the absence of a `RouteSource`. Derive dependencies from what the block displays and relies on, including inherited inputs, and track missing attribution by kind. `docs/phases/phase-9/design.md:185`

2. **Architecture · `Parse(r)` cannot establish the completeness E14 requires.** A dump filtered by name or field mask can still contain a valid `RoutesConfigDump` with no skipped entries. The same bytes can represent complete or filtered input. Carry capture completeness into the API, default unverified offline inputs to `unknown`, and test filtered and draining cases alongside the complete case. `docs/phases/phase-9/design.md:57` · `docs/phases/phase-9/implementation-plan.md:60`

3. **Architecture · Slot-only render hooks lose route context.** The same HCM slot is printed under multiple routes, which can have different policy links and deciding configs. `Style` and `Notes` receiving one ID cannot identify which row to highlight, expand, or annotate. Use the `(route ID, slot ID)` pair for filter rows and retain a route hook for kgwctl’s narrowing note. Test one slot under two routes with different configs and contributions. `docs/phases/phase-9/design.md:249` · `internal/renderer/renderer_interactive.go:55`

4. **Correctness · The prune rule drops active listeners that are mid-update.** The design says to retain selected routes, ancestors, and non-active listeners. Today an active listener also survives narrowing if it is warming a replacement, draining an older version, or has a rejected update. kgwctl already tests the mid-update case. Preserve that behavior explicitly in R8 and 9.1. `docs/phases/phase-9/design.md:83` · `pkg/envoy/filter/filter.go:302` · `kgwctl/internal/app/proxy_routes_test.go:632`

### P2

5. **Architecture · Claim-wide evidence and subject/kind resolution conflate distinct facts.** A name like `listener~80` supports the port, while the Gateway ref comes from caller context. A nonempty evidence slice lets the combined claim pass rule 5. Likewise, two exact policy records on one route are cumulative facts, yet the proposed resolver calls them ambiguous. Separate caller context from object assertions, give facts independent certainty, and define which assertions are alternatives and which are lists. `docs/phases/phase-9/design.md:162` · `docs/phases/phase-9/design.md:174`

6. **Architecture · Filter links lack a reliable record reference.** Modules emit links before `Compose` assigns final claim IDs. Repeating a module name can collide under `<module>/<n>`, and the validation rules allow dangling links or links to a slot on another HCM. Define module-local record references, have the composer resolve and validate them, and test duplicates, rejected records, and wrong-HCM slots. Preserve the existing requirement for applicable per-filter config, including weighted-backend overrides. `docs/phases/phase-9/design.md:171` · `pkg/envoy/chain/annotate.go:66`

7. **Architecture · RouteConfiguration source identity is underspecified.** The design gives sources `RD`/`RS` IDs but does not show a source-ID reference on each use, define inline-source IDs, or let an unattached source carry a `PolicyRecord`. The OSS readiness dump contains `main_route` both inline and in `static_route_configs`. Specify the source/use relationship, keep static and inline entries out of dynamic RDS name lookup, and test scoped metadata on a source with zero and multiple uses. Also state whether the HCM and its network filter intentionally share an ID. `docs/phases/phase-9/design.md:81` · `testdata/scenarios/00-simple/envoy/kgateway-v2.5.0/config_dump.json:4110`

8. **Architecture · Selection lacks the outcomes both consumers already use.** A route with a recognizable name marker but no valid reading cannot appear in kgwctl’s query-specific unresolved list through `Resolve` alone. Conversely, krp uses `Known` in live mode without `--route`, when the proposed flow never calls `Select`. Define both outcomes and preserve kgwctl’s rule that `Known` alone cannot establish a match. `docs/phases/phase-9/design.md:200` · `docs/phases/phase-9/design.md:251` · `kgwctl/internal/proxyview/view.go:114` · `cmd/krp/main.go:243`

9. **Correctness · The execution tests leave Envoy semantics ambiguous.** ECDS needs a rule for identifying the applicable resource and handling `default_config`, including `apply_default_config_without_warming`. `Conditional` must mean matcher-dependent, since a match can trigger `SkipFilter`. The matcher/action checklist also misses top-level `exact_match`, `prefix_match`, `suffix_match` and `contains_match`, `connect_matcher`, and inline cluster specifier plugins. Require text and JSON to retain unsupported semantics explicitly, and show unresolved filters in position with an uncertainty label. `docs/phases/phase-9/design.md:96` · `docs/phases/phase-9/design.md:126` · `docs/phases/phase-9/implementation-plan.md:63`

10. **Correctness · The green checkpoints and E15 acceptance are incomplete.** In 9.1, duplicate-RDS handling changes observable text despite the unchanged-text gate. In 9.2, removing `chain.Annotate` and changing the renderer requires migrating attribution tests and `internal/renderer`/`tui` callers in the same step. In 9.3, define which chains, source data, claims and warnings filtered JSON emits. Serializing the full overlays can expose pruned routes and produce references to absent subjects. Add a consumer acceptance check for kgwctl’s own payload, shared JSON/YAML envelope bump, and text/JSON/YAML goldens. The plan also allows implementation before consumer review, while D-019 and program question 7 put both reviews before design approval. `docs/phases/phase-9/implementation-plan.md:23` · `docs/phases/phase-9/implementation-plan.md:80` · `kgateway-tooling/program.md:47`

### P3

11. **Correctness · Contract tests need broader inputs.** Rule 2 calls for independent parses under every supported composition. Test a real two-module composition and mutation of nested maps, slices and pointers. R9 tests omit listener names, and Snellius’s current `Object(name)` seam lacks the `NameKind` argument, so exercise the stats adapter with typed inputs and ambiguity preserved. P-012’s table membership checks catch missing entries but cannot catch a wrong side or field mapping after a version bump. Tie those checks to versioned behavioral fixtures or document the fallback. `docs/phases/phase-9/implementation-plan.md:34` · `docs/phases/phase-9/implementation-plan.md:42` · `snellius/pkg/computed/envoynames.go:8` · `kgateway-tooling/program.md:232`

**Coverage check.** Scope items 1–8 map to R1, R2, R4, R3, R5/R6, R8, R9 and R10 respectively. Each has a design section and a plan test, with the partial coverage above. All nine core-correctness rows have test entries. P-010 rules 1–4 and 6 are explicit, rule 5 needs per-object enforcement, and rules 7–8 are partly carried by D-019/D-020 and the close plan. Make the post-release break approval rule and the downstream module’s public-API-only check explicit. P-011 permits the snapshot-local ordinals. `map[model.ID]...` is valid Go, and a package-level `Resolve[T Assertion]` can use a sealed interface as its constraint.

The author reports `go test ./...` and `go vet ./...` passing on `40822d0` plus documentation. I reviewed the design and plan without running tests or editing files.
## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | `Dependencies` misses inherited inputs; gaps not per kind | architecture | accept: design §5 `Dependencies` and `Gap`; plan 9.3 test 7 |
| 2 | Completeness can't come from the bytes | architecture | accept: `parser.Options.Capture`, `Snapshot.Capture`; §4.1, plan 9.1 test 9, 9.4 test 1 |
| 3 | Slot-keyed render hooks lose route context | architecture | accept: `render.Row`; plan 9.2 test 4 |
| 4 | Prune drops active listeners mid-update | correctness | accept: verified at `filter.go:302`; §2 and plan 9.1 test 6 |
| 5 | Claim-wide evidence; alternatives vs cumulative | architecture | accept: caller context out of claims (`Options.Gateway` gone), cardinality column |
| 6 | Filter link record reference and validation | architecture | accept: `Claim.Ref`, composer resolves links, compose rule 4; plan 9.3 test 2 |
| 7 | Route config source and use underspecified | architecture | accept: §2 sources and uses, inline `RCI`, `Reported`; plan 9.1 test 8 |
| 8 | Selection lacks unresolved and `Known` without query | architecture | accept: `Selection.Unresolved`, `Narrow`; plan 9.3 test 8 |
| 9 | ECDS, `Conditional`, missing matchers and specifiers | correctness | accept: §3 `ConfigSource`, §4 rows 4, 5, 7 |
| 10 | Green checkpoints, filtered JSON, consumer acceptance, review order | correctness | accept checkpoints, projection rule and example test (plan 9.1, 9.2, 9.3 test 11–12). Review order: owner decision, recorded in plan "Before 9.3 locks the API" |
| 11 | Broader contract tests; table checks catch membership only | correctness | accept: two-module mutation test, listener names, `Guess`, R14 fallback, R15 |
