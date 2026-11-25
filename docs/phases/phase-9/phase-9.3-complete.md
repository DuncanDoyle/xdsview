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
