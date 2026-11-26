# Sub-phase 9.4 complete — core correctness

**Date:** 2026-11-25 | **Kind:** planned | **Base:** ce17778 | **Commits:** 40c202c, 469c4f3, 9659a18, f7d1a28, 5640b2a, 1f411b8, a12be5a, 9eb9ace, 93128af

## What changed, per item
| # | Item | Change |
|---|---|---|
| 1 | Static listeners, inline source (review 9.1–9.2 #5) | `static_listeners` parsed after the dynamic ones, `Listener.Static`, always active. Scenario 00's `main_route` is one `<HCM>/RCI` source with `Reported: RS…`. Text marks `· static`. |
| 2 | Non-HCM chains | `NetworkFilterChain.NetworkFilters` lists every network filter (`L<n>/C<m>/N<k>`, name, type); the HCM keeps its filter's ID and is found by type as well as name. Text lists them for a chain without an HCM or with another network filter. |
| 3 | Default filter chain | Parsed as the listener's last chain, ID `L<n>/Cd`, `Default: true`. Text: `default chain <name>`. |
| 4 | Missing RDS (E14) | `missing` and `rds-missing` only on a complete capture, with a RoutesConfigDump read in full and an active listener; else `unknown`. Text: `route config X missing: …`. |
| 5 | ECDS | `HTTPFilter.Discovery`. Entry by filter name with its type in `type_urls` → `ecds`; none, active listener, `default_config` → `ecdsDefault` (either warming flag); else `Runs` unknown, `State` `unresolved`, `?` in text. `ecds-ambiguous`, `ecds-skipped` warnings. |
| 6 | Matchers | Every top-level header form, every `string_match` form (incl. `custom`), `invert_match`, `ignore_case`, `treat_missing_header_as_empty`; no specifier = present. Query `string_match`/`present_match`. `CaseSensitive`, `Connect`. Other match fields → `RouteMatch.Unparsed`, in text and JSON. |
| 7 | Cluster selectors | `Route.Action` (`Kind`, `ClusterHeader`, `ClusterSpecifierPlugin`, `InlineClusterSpecifierPlugin`); `Cluster` stays empty; text `backend: from …`. |
| 8 | Route actions | `Kind` route, redirect, directResponse, filterAction, nonForwarding. Redirect keeps status (301 default) and location parts; direct response keeps status, body size or source, never the body. |
| 9 | Coverage | `Snapshot.Coverage.Sections`: parsed, partial, ignored, in dump order. Ignored sections are read for `@type` only. `scoped_routes` → `unknown`, `HCMConfig.ScopedRoutes`. Text ends with `ignored dump sections: …`. |

## Tests
Each item's tests were written first and failed for the expected reason, then passed (before and after output: local captures, not published). Parser tests in `parser/correctness_test.go`, chain in `chain/ecds_test.go`, krp text and JSON in `cmd/krp/correctness_test.go`. Before, in short:
1. Fixture: no listeners; scenario 00: `RS0`/`RS1` unused, no inline source.
2. `NetworkFilters` empty; text `└─ no HTTP connection manager` only.
3. Only `L0/C0`; default chain absent from text.
4. Complete capture still `unknown`, no warning; text `not found`.
5. All ECDS filters `on`, `ConfigSource: inline`, no type: shown as running.
6. Top-level forms printed `header(x-prefix=)`; no `Kind`, `Connect`, `Unparsed`.
7. Selector routes printed no backend.
8. Redirect and direct-response routes printed nothing about the action.
9. No coverage; scoped HCM printed `route config  not found`.

After: `go test ./...` and `go vet ./...` pass.

## Golden changes
Reviewed after each `-update`; no line was removed except a trailing comma or the item-1 source swap.
- Item 1: every corpus text and JSON golden gains `readiness_listener` and `prometheus_listener`, except `.route`/`.rule0`/`.nomatch`, where the filter drops them. In all JSON goldens, static sources `RS0`/`RS1` give way to inline sources with `reported`; the filtered ones keep these sources, since `filter` doesn't prune sources (as in 9.1).
- Item 2: JSON gains `networkFilters` on every chain.
- Item 6: JSON header and query matches gain `kind`.
- Item 7: JSON routes gain `"action": {"kind": "route"}`.
- Item 9: JSON gains `coverage`; text gains the `ignored dump sections` line.
- New: text and JSON goldens for fixtures `static-bootstrap`, `tcp-proxy`, `default-chain`, `ecds`, `matchers`, `cluster-selectors`, `route-actions`, `coverage`; `upstream-extproc.txt`, since the text golden test now covers fixtures.

## Deviations
1. **Static listeners follow the dynamic ones**, not dump order, so dynamic IDs (`L0`…) don't move when a bootstrap changes.
2. **Default chain lives in `FilterChains`** (last, `Default: true`), not in a `Listener.DefaultFilterChain` field: chain, prune, filter and render see it unchanged.
3. **HCM found by type** too: Envoy picks the factory by `typed_config` type.
4. **ECDS:** a filter the HCM disables and the route doesn't enable is `Runs: no` even unresolved. Two matching entries → unresolved plus `ecds-ambiguous` (no first-wins). `TypedStruct` configs aren't unwrapped.
5. **Missing RDS is conservative:** any skipped route config entry, static ones included, or an unreadable section makes it `unknown`.
6. **Coverage:** a section that fails to decode is `partial`; one with an unreadable `@type` is `ignored` with an empty type.
7. **`HeaderMatch.Regex` and `QueryParamMatch.Regex` stay** beside `Kind` for `v1alpha1` readers.
8. **Health check joins the sides table** (`health_check/config.cc:58`); scenario bootstrap listeners use it.
9. Corpus tests that walk kgateway routes skip `Static` listeners.

## For 9.3
- Every corpus dump now has two static listeners. The kgateway mapper gives them no origin; `filter.Filter` drops them under `--route`. `Narrow`/`Select` should do the same, or say why not.
- New additive model fields for `v1alpha2`: `Static`, `NetworkFilters`, `Default`, `Discovery`, `ScopedRoutes`, `Action`, `Coverage`, matcher `Kind` and modifiers. Decide there whether `Regex` and `Route.Cluster` stay.
- ECDS filters now carry their resolved `TypedConfig`, so filter attribution may start to match them.
- `schemaVersion` is still `v1alpha1`.

## Review fixes
Codex review [9.4-codex.md](reviews/9.4-codex.md), triage there. Each test failed first, then passed (local captures, not published). `go test ./...` and `go vet ./...` pass.

| # | Commit | Change |
|---|---|---|
| 1, 9 | [`c55dd05`](https://github.com/DuncanDoyle/xdsview/commit/c55dd05) | ECDS entries join per category (downstream, upstream HTTP) and need one config source per name. Sections map to categories by Envoy's key order only when every `config_discovery` in the listeners is accounted for and the section count matches; otherwise a found entry leaves the slot unresolved with `ecds-ambiguous`. |
| 2 | [`ed002e2`](https://github.com/DuncanDoyle/xdsview/commit/ed002e2) | A network filter with `config_discovery` gets `NetworkFilter.Discovery`, unresolved; text: `(ECDS, unresolved) · HTTP filters unknown`. An ECDS router's upstream filters come from the config it runs. |
| 3 | [`71ab11d`](https://github.com/DuncanDoyle/xdsview/commit/71ab11d) | `ecdsDefault` needs `CaptureComplete` and no skipped ECDS entry or section. |
| 4 | [`ba0f9fc`](https://github.com/DuncanDoyle/xdsview/commit/ba0f9fc) | Missing RDS on a static listener needs an active dynamic listener (workers started); else `unknown`. ECDS defaults follow the same rule. |
| 5 | [`c558772`](https://github.com/DuncanDoyle/xdsview/commit/c558772) | The default chain has no `TLS`: Envoy ignores its `filter_chain_match`. |
| 6 | [`88b0e08`](https://github.com/DuncanDoyle/xdsview/commit/88b0e08) | Empty top-level `exact_match` → new `MatchKind` `anyValue`, text `any value`. Not `present`: with `invert_match` the two differ (`header_utility.h`). |
| 7 | [`266b517`](https://github.com/DuncanDoyle/xdsview/commit/266b517) | A reported static entry stays a source; `Prune` keeps the entry a kept source reports. No `reported` ID dangles. |
| 8 | [`59ce072`](https://github.com/DuncanDoyle/xdsview/commit/59ce072) | `RouteConfigUse.Chain` (`effectiveChain`) carries the chain of an HCM with no route config in JSON. |

Golden changes, reviewed after each `-update`:
- `88b0e08`: `matchers.txt`/`.json` gain the fixture's new `x-any` header (`any value`, `kind: anyValue`).
- `266b517`: all 55 corpus JSON goldens and `static-bootstrap.json` gain the static sources `RS0`/`RS1` in `routeConfigs`; nothing removed.
- `59ce072`: `coverage.json`: the scoped-routes use gains `effectiveChain`.
- `71ab11d`: `ecds.txt`/`.json`: `ecds.default` and `ecds.default_no_warming` become unresolved (`?`), since goldens parse files as unverified.
- `ed002e2`: new `ecds-network.txt`/`.json`.
- `c558772`, `ba0f9fc`, `c55dd05`: none. The `default-chain` fixture gains an SNI on its default chain; output is unchanged.

Golden changes item 1 no longer holds: static sources stay beside the inline ones that report them. Scope limits that stay:
- Cluster upstream filters share the router's ECDS subscriptions but aren't read. A cluster subscribing to a router filter's name from another config source isn't detected.
- An ECDS router with an entry, or any `config_discovery` outside HCM and router filters, leaves sections unmapped, so found entries stay unresolved.
- An unresolved ECDS router's upstream filters are unknown and not listed; the router slot's `?` covers them.
- A static listener's `Status` stays `active` even before workers start; only the missing-RDS and ECDS-default verdicts check.

For 9.3: `Prune` keeps reported sources; `RouteConfigUse.Chain` is a provenance-free copy like `Route.Chain`, to move with it.
