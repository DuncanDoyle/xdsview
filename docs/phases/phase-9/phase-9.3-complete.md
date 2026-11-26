# Sub-phase 9.3 complete — provenance contract and kgateway module

**Date:** 2026-11-25 | **Kind:** planned | **Base:** 3ea6044

9.3 runs in two parts. Part A adds the contract and the module beside today's code. Part B ports krp, moves JSON to `v1alpha2` and deletes `origin`, `filter` and the model provenance fields.

## Part A

**Commits:** [`07e612e`](https://github.com/DuncanDoyle/xdsview/commit/07e612e), [`0eea752`](https://github.com/DuncanDoyle/xdsview/commit/0eea752), [`bc5b8e2`](https://github.com/DuncanDoyle/xdsview/commit/bc5b8e2), [`a168ae0`](https://github.com/DuncanDoyle/xdsview/commit/a168ae0), [`1f3af2f`](https://github.com/DuncanDoyle/xdsview/commit/1f3af2f), [`62115ba`](https://github.com/DuncanDoyle/xdsview/commit/62115ba)

### What exists

| Package | Added |
|---|---|
| `model` | `Listener.Metadata`, `VirtualHost.Metadata`, `RouteConfigSource.Metadata` and `TypedPerFilterConfig` (review 9.1–9.2 #3); `SourcePath` on vhosts and routes; lookups `Listener`, `FilterChain`, `HCM`, `VirtualHost`, `Route`, `WeightedCluster`, `Slot`, `RouteConfigSource`; `CompareID`; `WarnClaimDropped`, `WarnLinkDropped`. Old provenance fields untouched. |
| `parser` | Fills the fields above. |
| `render` | `Options.Trailer func(Row) []Line`: lines after a route's filters and upstream filters line, before its actions and backend (review #4). |
| `provenance` | `Claim`, `Evidence`, `EvidenceKind`, sealed `Assertion` with `Kind()` and JSON (`assertion: {kind, …}`), the eight kinds, `Level`, `Module`, `Compose`, `Index` (`About`, `Claims`, `Record`, `Warnings`, `Dependencies`), `Resolve[T]`, `Resolution`, `Dependencies`, `Dependent`, `Gap`, `Records`, `Narrowing`, `NameMapper`, `NameKind`, `NameAnswer`, `Guess`, `Labels`, `Notes`. |
| `adapter/kgateway` | `New`, `Select`, `Narrow`, `Query`, `Selection`, `Narrowing` (alias), `Names`, `CheckedMinors`, `PolicyTableVersion` (`v2.5.1`), `PolicyFields`. |

Core packages still import only the standard library and each other (`go list -deps`, `TestPureCoreImportsNoProvenance`).

### Tests

Before: `provenance` and `adapter/kgateway` didn't exist, so every contract test failed to compile. The P1 cases fail on today's code; `TestParity_P1Fixes` asserts that. With the per-module copy made shallow, `TestCompose_Invariance` fails on all 61 dumps (snapshot and chains changed) and on 52 (kgateway claims changed).

After: `go test ./...` and `go vet ./...` pass.

| Plan §9.3 | Test |
|---|---|
| 1 Invariance | `provenance` `TestCompose_Invariance` (every dump; `vandal` mutates maps, slices, pointers) |
| 2 Compose rules | `TestCompose_Rule3`, `_DuplicateModules`, `_Alternatives`, `_Rule4`, `_Order`, `TestClaim_JSON` |
| 3 Claims parity | krp `TestParity_Text`, `TestParity_Structured` (every dump and fixture) |
| 4 `merge.*` levels | `kgateway` `TestMergeLevels`, hand-built: no kind capture was tried |
| 5 Filter link | `TestFilterLink`; `TestCompose_Rule4` for unrelated listener and unnamed object (`TestCompose_Rule3`) |
| 6 Deferred P1s | `TestP1Fixes`, krp `TestParity_P1Fixes`, fixture `snaptest.P1` |
| 7 Dependencies | `TestDependencies`, `TestDependencies_PolicyGaps`, `TestRecords` |
| 8 Selection | `TestSelect`, `TestNarrow`, krp `TestParity_Selection` |
| 9 NameMapper | `TestNames`, `TestGuess` |
| 10 Policy table | `TestPolicyTable_CoversCorpus`: corpus fields cors, extAuth, extProc, rateLimit.global, rateLimit.local, transformation; no exclusions |
| 12 Consumer payload | `Example_routePayload` |

Also: `model` `TestCompareID`, `TestSnapshot_Lookups`; `parser` `TestParse_LevelMetadata`; `render` `TestText_Trailer`; `provenance` `TestText_RowsPerRoute`; `kgateway` `TestStaticListeners`.

### Parity

On every corpus dump and fixture, the module's resolved listener, vhost and route origins, route policies and filter contributions equal `origin.Annotate` plus krp's `attribute`, and the text from `Labels` and `Notes` equals today's. Selection equals `filter.Filter` and `filter.Ambiguous` for nine `--route`/`--rule` queries. The only intended differences are the P1 fixes; the corpus shows none of them.

### Golden changes

JSON goldens (63 files) gain `sourcePath` on every vhost and route. Nothing else changed; no corpus dump has metadata above route level.

### Deviations

1. **Policies line.** `provenance.Notes(ix)` returns a `render.Options.Trailer` hook (`[]render.Line`), not an `Options.Notes` one. Route `Notes` print before the filters; the policies line prints after them. It adds an `inherited policies:` line for records above route level; the corpus has none.
2. **`ListenerSource` on a virtual host.** Today's vhost text comes from the listener part of the vhost name. The vhost is an allowed subject beside listener, filter chain and source.
3. **`NameRoute` kind added.** kgateway route names are its only ambiguous names. Route config and listener names carry no object without caller context, so they answer unknown. ListenerSet names aren't read.
4. **`Narrowing` lives in `provenance`.** `Labels` needs it, and `provenance` can't import the module; `kgateway.Narrowing` is an alias.
5. **`Selection.Narrowed`** lists only routes in the query's buckets, not every route Known narrowed.
6. **krp `--route` keeps today's routes only with its own rule.** Today krp passes `--route` as Known. Corpus route names read two ways, so rules 3–4 put them in `Ambiguous` with a one-reading `Narrowing`. `TestParity_Selection` reproduces today's routes as `Routes` plus narrowings that kept one matching reading (`krpRoutes`).
7. **Rule 4 duplicates** count only links that pass the other checks, so an invalid link can't cancel a valid one.
8. **Rule 3 also drops** a claim on the wrong kind of subject, a `PolicyRecord` whose `Level` doesn't match its subject, and a repeated `Ref`.
9. **Gaps.** An `extensionSource` gap needs per-filter config for the slot on any route of its HCM. `gatewaySource`, `routeSource` and default-on `policyRecord` gaps apply to static listeners too, which have no claims. A `FilterLink`'s metadata evidence names the policy, so `Dependencies` lists it under `filterLink`.
10. **`Unresolved`** means a route-name marker with no `RouteSource` claim. `filter` selects such a route by substring today; no corpus route is one.
11. **P1 rules.** #5: any Composite delegate is a candidate, not only a sole one. #4: with `dynamic_module_config.module` set, a link needs a local file named `librust_module.so`; a remote module gets none.
12. **Gateway.** `GatewaySource` comes from metadata only, and kgateway writes none. Listener text loses `Gateway ns/x` under `--gateway` once krp is ported.
13. **Claim JSON** nests the assertion (`assertion: {kind, …}`, additions 2), not `kind` at claim level as the §7 sketch shows.

### For part B

1. Port krp: parse → `chain.Effective` → `Compose(snap, kgateway.New())` → `Select` (with `--route`) or `Narrow` (live) → `model.Prune` → render with `Labels`, `Notes` as `Trailer`. Decide deviation 6 and keep the text goldens.
2. Move `extraMappers` and `TestDumpJSON_PluginOrigin` to a test `provenance.Module`.
3. JSON `v1alpha2`: `chains`, `claims`, `selection` with route and vhost names, typed warnings, projection rule; schema doc and README.
4. Port `internal/renderer`, the TUI and `render`'s tests (they build `sample()` with `origin.Annotate`).
5. Delete `origin`, `filter`, `cmd/krp/attribution.go`, `cmd/krp/parity_test.go`, the model fields `Listener.Origin`, `VirtualHost.Origin`, `HTTPFilter.Origin`, `Route.Origin`, `Route.Policies`, `Route.Chain`, `RouteConfigUse.Chain`, the `*Origin`, `PolicyRef`, `Contribution` types, and render's code that prints them.
6. Decide on `HeaderMatch.Regex`, `QueryParamMatch.Regex` and `Route.Cluster` (9.4 note).
7. Update phase-status.md; the close benchmark includes `Compose`.

## Part B

**Commits:** [`207ceab`](https://github.com/DuncanDoyle/xdsview/commit/207ceab), [`442b7fa`](https://github.com/DuncanDoyle/xdsview/commit/442b7fa), [`a4ab9b3`](https://github.com/DuncanDoyle/xdsview/commit/a4ab9b3)

### Changes

| Area | Change |
|---|---|
| krp | `build`: parse → `chain.Effective` → `Compose(snap, kgateway.New(), extraModules...)` → `Select` under `--route` (Known = the `--route` HTTPRoute) or `Narrow` in live mode → `model.Prune` → render with `Labels` and `Notes` as `Trailer`. D-022: also selects a narrowed route whose one kept reading matches the query; `--gateway` prints on non-static listener lines as `Gateway ns/gw (from --gateway)`. Index warnings go to stderr. |
| krp `-o json` | `v1alpha2`: `snapshot`, `chains`, `claims`, `selection` (`query`, `routes`, `ambiguous`, `unresolved`, `narrowed`; entries carry `name` and `virtualHost`; D-022 routes `"selected": true`), typed `warnings`. Everything but `selection` is projected onto the retained subjects. |
| `internal/renderer`, TUI | `View.Trailer`; the TUI reads the same `View`. |
| `render` | No longer prints model origins or `Route.Policies`. |
| `model` | Removed `Listener.Origin`, `HTTPFilter.Origin`, `VirtualHost.Origin`, `Route.Origin`, `Route.Policies`, `Route.Chain`, `RouteConfigUse.Chain`, the `*Origin`, `RouteAnnotation`, `PolicyRef` and `Contribution` types. `origin.go` is now `ref.go` (`Ref`, `Certainty`). |
| Deleted | `pkg/envoy/origin`, `pkg/envoy/filter`, `cmd/krp/attribution.go`, `cmd/krp/parity_test.go`. |
| Docs | README: JSON field table (the schema doc), D-022 behaviour, library example on `provenance` and `kgateway`, project structure. |

### Tests

Before deleting the oracle, `TestParity_Krp` ran krp's old path (`filter.Filter`, `origin.Annotate`, `attribute`) and `build` on every dump and fixture in file mode, live mode with the corpus HTTPRoutes, and nine `--route`/`--rule` queries: 671 outputs, text identical. It went with `parity_test.go`.

| Plan §9.3 | Test |
|---|---|
| 3 Claims parity | `TestParity_Krp` (above); `TestText_Golden` now runs `build`, goldens unchanged |
| 8 Selection | `TestDumpText_Origins`, `TestDumpLive_*`; `TestDumpJSON_Projection` checks a D-022 route is `selected` exactly when it is in `routes`, and that some are |
| 11 krp `v1alpha2` | `TestDumpJSON_Projection`: every dump × five `--route` queries; no ID outside `selection` (chain keys and slots, claim and evidence subjects, link slots, warning subjects, `reported`, use sources) is absent from `snapshot`; selection entries carry names. Without projection it reports 6,440 violations. `checkEnvelope`, `TestDumpJSON_Warnings` on the new shape. |

Also: `claims_test.go` ports the attribution tests to `build`, reading the filter-row labels; vendor plugins are test modules (`linker`, `extension`). `TestDumpJSON_PluginOrigin` uses a test `Module` whose exact `extensionSource` wins over kgateway's. `provenance` `TestText_Formats` takes the label formats `render` tested. `TestNothingImportsRemovedPackages` and a third-party check in `TestPureCoreImportsNoProvenance`.

`go test ./...` and `go vet ./...` pass. `go list -deps` of `model`, `parser`, `chain` and `render` lists only those four besides the standard library.

### Golden changes

Text (61 files): none.

JSON (65 files), all for the `v1alpha2` shape: `snapshot` loses `origin`, `policies`, `effectiveChain` and `warnings`; `chains`, `claims` and `warnings` (typed) are new, and `selection` under `--route`. Under `--route` (`.route`, `.rule0`, `.nomatch`), `snapshot.routeConfigs` drops the inline sources of the static listeners, which `model.Prune` removes with their listeners and `filter.Filter` kept. The selected routes are the same. `.plugin` shows the test module's claims beside kgateway's.

### Deviations

1. **Warnings move.** `snapshot.warnings` is empty in JSON; the envelope's `warnings` holds the parse warnings and the index's. An index warning whose subject the dump lacks keeps its message without a subject.
2. **`selection` presence.** Absent in file mode without `--route`; live mode without `--route` has `narrowed` only. `query` is added.
3. **`selection.ambiguous`** leaves out the routes krp selects by D-022; `Select.Ambiguous` includes them. They are in `routes` and in `narrowed` with `selected`.
4. **`narrowed` embeds the kept and dropped claims**, not claim IDs: `claims` is projected, so IDs could dangle.
5. **D-022 Gateway text** follows the listener's own label: `port 80 (inferred) · Gateway ns/gw (from --gateway)`, where krp printed `Gateway ns/gw port 80 (inferred)`. No golden uses `--gateway`.
6. **No separate schema doc.** The README JSON section is the schema doc; it lists each assertion kind.
7. **Dropped tests.** The attribution cases for a mapper without `PolicyFilters` and for no mapper (the interface is gone; krp always composes kgateway); `TestAttribute_RowsPerRoute` and render's `TestText_KnownWording` (covered by provenance `TestText_RowsPerRoute`).
8. **Process.** One edit to `main_test.go` used `perl -pi` instead of the Edit tool.

### For the phase review

1. Part A item 6 (`HeaderMatch.Regex`, `QueryParamMatch.Regex`, `Route.Cluster`) is not decided.
2. The close benchmark needs `Compose` on the largest dump.
3. Codex review of 9.3 (parts A and B).
4. `.agents-tmp/tuicheck` still calls the removed API; it is scratch and outside the build.

## Benchmark

Close benchmark (Codex review #8), in `pkg/adapter/kgateway/bench_test.go`. The dump is synthetic: one listener, 50 vhosts × 100 routes (5,000), each with `route_source` and `merge` metadata and per-route ext_authz config; about 2.5 MB, generated in the benchmark. Apple M5 Max, Go 1.27.4, `go test ./pkg/adapter/kgateway/ -run '^$' -bench . -benchmem`:

| Benchmark | Time/op | Memory/op | Allocs/op |
|---|---|---|---|
| `parser.Parse` | 46.9 ms (54 MB/s) | 64.3 MB | 490,094 |
| `chain.Effective` | 2.5 ms | 7.6 MB | 15,048 |
| `provenance.Compose` (kgateway module) | 39.1 ms | 85.3 MB | 590,063 |

Fail-closed `Dependencies` (Codex re-review 4), rerun on the same machine. `BenchmarkDependencies` composes once and checks all 5,000 route blocks:

| Benchmark | Time/op | Memory/op | Allocs/op |
|---|---|---|---|
| `provenance.Compose` (kgateway module) | 39.8 ms | 85.3 MB | 590,066 |
| `Compose` + `Dependencies` on every route block | 67.1 ms | 135.0 MB | 1,032,906 |

`Dependencies` on all route blocks costs about 27 ms (5.5 µs and 89 allocations per block). The first call builds the per-HCM structure and link placements, so the cost is linear in routes.

Not measured: the kind capture for scoped `merge.*`; that is for the owner (R5 tested on hand-built fixtures, design Risks).

## Corpus gaps

Fail-closed `Dependencies` on the 279 route blocks of the 51 scenario dumps (kgateway module only, file capture). 204 blocks sit on the static `readiness_listener` and `prometheus_listener` (51 dumps each); 75 sit on kgateway listeners. "Bootstrap" is the count once static listeners became bootstrap inputs (§5): all 204 static blocks are now gap-free, and none of the 75 kgateway blocks is.

| Gap kind | Before | After | Bootstrap | Static | kgateway | Reason (`Inputs`) | Class | Note |
|---|---|---|---|---|---|---|---|---|
| `gatewaySource` | 279 | 279 | 75 | 204 | 75 | `noEvidence` | b | No corpus listener has metadata. A consumer may close a kgateway listener's gap with its captured Gateway (§5 note 2). |
| `routeSource` | 271 | 271 | 67 | 204 | 67 | `ambiguous` | b | kgateway: no `route_source` metadata, and the name reads two to five ways; krp prints `ambiguous` too. The 8 routes with metadata resolve. |
| `hostSource` | 0 | 204 | 0 | 204 | 0 | | b | Bootstrap vhosts `local_service`, `prometheus_host`; kgateway vhosts all resolve. |
| `backendSource` | 204 | 204 | 0 | 204 | 0 | | b | `admin_port_cluster` is bootstrap; kgateway backends all resolve. |
| `backendSource` (mirror) | 0 | 2 | 2 | 0 | 2 | `notCoverable` | a | `httpbin-mirror` in both `02_4-single-policy` dumps. No Phase 9 kind covers a mirror (§5). |
| `extensionSource` | 0 | 51 | 0 | 51 | 0 | | b | `envoy.filters.http.health_check` on the readiness listener. |
| `policyRecord` | 51 | 51 | 0 | 51 | 0 | | b | Same slot runs by default at listener level, with no `merge.*` record. |

One (a) cause, now fixed: mirror clusters, which both renderers show, were no input, so a route could hide a mirror Service the viewer cannot read. The JSON oracle test also found per-request cluster selectors, non-HCM network filters and a reported static source's listener missing; none occurs on a scenario route block. Static inputs are still enumerated, with status `bootstrap`, so the oracle and fail-closed tests keep covering them.

(c), not on the corpus: `placement` reports a gap when the routes checked are empty, as for a default-on slot of an HCM with no routes. §5's "for each route checked" would hold vacuously there. This predates the fail-closed rewrite.
