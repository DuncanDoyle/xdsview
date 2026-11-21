# Phase 6.3 — `pkg/envoy/render` and krp (Complete)

**Completed:** 2026-11-21
**Range:** 371297e..6713dd7 (`main`)

## Summary
`render.Text` writes the snapshot as a deterministic plain-text tree using only the standard library and the `pkg/envoy` core. By default each route lists its running filters in request order and one `not run:` line. `AllFilters` lists every filter inline, and `ResponseOrder` adds the response order. `krp` text output is that tree, coloured through `Style`, with no bordered panel. The TUI renders with `AllFilters`, highlights its cursor through `Style`, and shows the deciding config through `Extra`. `krp dump` annotates chains for text and `-o json`.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Design text shape | `TestText_Default` checks the full output of a synthetic Gateway literally. It covers a running filter enabled per route with its contributions, a not-run line, upstream filters, policies, request headers, the backend, and a warming listener with an error. |
| Options and cases | `TestText_AllFilters`, `TestText_ResponseOrder`, `TestText_UnknownSide`, `TestText_Conditional`, `TestText_ListenerStates` (including an LDS error), `TestText_RouteOrigins` (`NarrowedByKnown` and ambiguous), `TestText_Unannotated`, `TestText_NoListeners` and `TestText_Hooks`. The first run failed only on route-body indent (one level too deep). After the fix all pass. |
| Corpus goldens | `TestText_Golden`: 50 `.txt` files in `testdata/golden/scenarios/<scenario>/<version>.txt`, with `-update`. Each case also renders twice and compares bytes, and checks for no `\x1b`, no trailing whitespace and a final LF. |
| Boundary | `render` is in `core`. `TestRenderHasNoCharmbracelet` passes. Mutation check: a `mutation.go` importing lipgloss in `render` made both boundary tests fail (four `github.com/charmbracelet/...` dependencies reported). With the file removed, they pass again. |
| `krp` text | `internal/renderer` maps `render.Class` to lipgloss styles over `render.Text`. The bordered panel is gone. |
| `krp -o json` | 72 goldens regenerated. With `effectiveChain` stripped, each equals HEAD under `jq -S`. |
| TUI navigation | `TestNavigation_EnabledDisabledEnabled` steps through a filter enabled by a vhost override, then an off filter, then an on filter. At each step it checks the highlighted row and the expanded config, and the vhost override's `FilterConfig` is the one shown. It would fail on the old renderer, which had no state text on these rows. |
| Manual TUI run | `krp dump -i --file testdata/scenarios/03-multi-policy/envoy/kgateway-v2.5.0/config_dump.json`, driven with `expect` in a 160×40 pty that answers the OSC 11 and cursor-position queries. Pressing down twice and then Enter moved the reverse-video cursor across rows 1, 2 and 3. The ext_auth row then showed its per-route `FilterConfig` (`"config": {}`). |
| Live kind check | kind + kgateway v2.5.0 + scenario `03`, plus a control HTTPRoute `/control` without policies on the same host. Envoy `aacd0340.../1.39.6/Clean/RELEASE/BoringSSL`. `krp dump --gateway gw` said ext_auth is `off` on `/control` and `enabled by route` on `/`. The oracle, `http.http_80.ext_authz.error`: 0 before, 0 after 3 requests to `/control` (404), and 3 after 3 requests to `/` (403). The cluster had its own kubeconfig and was deleted on exit. |
| tests / vet | `go test -count=1 ./...`: 12 packages ok. `go vet ./...` clean. |

**Tests changed by the layout:**
1. `internal/renderer/accuracy_test.go`:
   - `TestRender_WeightedClusters`: `Backends:` became `backends:`.
   - `TestRender_PrefixRewrite`: `rewrite-prefix:` became `rewrite prefix:`.
   - `TestRender_UpstreamFilters`: now checks one ordered line.
   - `TestRender_Origins`: the filter row and the `policies:` line. `renderRoute` now annotates chains as krp does.
   - `TestRender_AmbiguousOriginHidden` became `…Labelled` (see Notes).
2. `internal/renderer/renderer_test.go`:
   - `HeaderModifier`, `Mirror` and `URLRewrite` drop the `Route Policies` label.
   - `EmptySnapshot` now reads `no listeners`.
   - `NilHCM` now reads `no HTTP connection manager`.
   - `NoOpts_RicherSnapshot`: the "interactive equals static" invariant no longer holds, because the TUI lists every filter. The test now checks both forms.
3. `internal/renderer/resolve_test.go`: the new signature, plus a vhost and route config level test.
4. `cmd/krp/main_test.go`: the `TestDumpText_Origins` filter row; `Route: ` became `─ route `; and the live test now checks for no resolved origin.
5. `pkg/envoy/parser/accuracy_e2e_test.go`: the upstream filters line.

The TUI tests pass unchanged. `TestDump_NoRoutesWarning` is new.

## Notes for later phases
These are layout choices the design leaves open:
1. **Numbering.** Rows are numbered within the printed list.
2. **Unknown side.** An unknown side is marked `N?` in both orders.
3. **Response-only filters.** A filter that runs only on the response side appears on a `response only:` line when the response order isn't printed.
4. **One filter chain.** A single filter chain without TLS has no tree level of its own.
5. **Ambiguous origins** print as `ambiguous: <candidates>`. Phase 5 printed nothing, but listing the candidates doesn't pick a wrong one.
6. **`policies:` line.** It stays alongside the contributions, because a policy field may not map to any filter.

Two behaviours changed in krp:
1. **krp keeps its narrowing order:** `filter.Filter`, then the annotations. Origin-based narrowing would use `--route` as its known-routes list and label the result "(inferred from cluster routes)". krp's narrowing is therefore unchanged. Consumers that annotate first get the new behaviour.
2. **The no-routes warning** counts routes. Listeners that aren't active survive narrowing, so counting listeners no longer works.

Open items:
- **Listener addresses** print as Envoy's address and port, joined with `:` (`:::80`). The design's `[::]:80` would need a parser change. Not done.

## Deferred / open
none
