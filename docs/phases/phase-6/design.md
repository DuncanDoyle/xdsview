# Phase 6 — Route View for Consumers: Design

**Base:** `40a5863`
**Roadmap:** `docs/roadmap.md` § Phase 6
**Program:** edge E11 (kgwctl Phase 3.2 `proxy routes`), request [#27](https://github.com/DuncanDoyle/xdsview/issues/27); the same model feeds E4 (Snellius Phase 12, runtime filter-chain tab)

## Goal

kgwctl and Snellius show a route's filter chain from xdsview's model and text, not from their own copies. The leading user question is filter order: does a transformation run before or after ext_authz, and does rate limiting see what it needs. In about nine of roughly 25 support tickets reviewed, the question was answered by reading `config_dump` by hand with jq.

1. **Effective filter chain per route.** Which HTTP filters run for one route, in request order and response order, and what turned each one on or off.
2. **Listener state.** Show warming, draining and rejected listeners and their error. A listener that is not active no longer renders as an empty listener.
3. **Exported text renderer.** `pkg/envoy/render` writes deterministic plain text using only the standard library. The `krp` CLI and TUI style that same output instead of keeping a second renderer.
4. **UI-neutral model.** New types live in `model`, so kgwctl and Snellius show the same facts.

## Non-goals

- Intended vs accepted config: moves to Phase 7 (D-008).
- Interpreting filter-specific per-route switches, such as `ExtAuthzPerRoute.disabled` or a `CorsPolicy` that turns CORS off. The entry carries the per-route config `@type`, so a consumer can show it, but xdsview does not judge it.
- Per-backend config in a traffic split (`weighted_clusters[].typed_per_filter_config`). Envoy picks it per request. The chain only flags that it exists.
- What happens inside a matcher or composite filter (`ExtensionWithMatcher`). The filter is shown as running, and its delegate type is shown when Phase 5 resolved it.
- Whether a filter handles requests, responses or both. Response order is the order in which response handlers would run.
- Styling, colour or terminal detection in `pkg/envoy`.
- `-o json` envelopes for consumers. kgwctl keeps its own; `krp dump -o json` only gains fields.

## Envoy semantics the chain follows

Sources:
- Envoy API docs for `HttpFilter.disabled`, `FilterConfig` and `typed_per_filter_config` on `RouteConfiguration`, `VirtualHost`, `Route` and `WeightedCluster.ClusterWeight`.
- The corpus. In v2.4.7 and v2.5.0, kgateway disables `ext_auth/*`, `ratelimit/*`, `envoy.filters.http.cors` and `global_disable/*` at HCM level, and enables them per route. It does that with a filter-specific config (`RateLimitPerRoute`, `CorsPolicy`) or with `FilterConfig{config: {}}`. The corpus has no per-filter config on a virtual host, a route config or a weighted cluster.

For each HCM filter `f`, xdsview takes the most specific `typed_per_filter_config[f.name]` entry: route over virtual host over route configuration. A weighted cluster's entry is not resolved (see Non-goals).

| HCM `disabled` | Most specific entry | Runs | `State` | `Level` |
|---|---|---|---|---|
| false | none | yes | `on` | — |
| false | `FilterConfig{disabled: true}` | no | `disabled` | where the entry is |
| false | any other | yes | `on` | where the entry is (config overridden) |
| true | none | no | `off` | — |
| true | `FilterConfig{disabled: true}` | no | `off` | where the entry is |
| true | any other, including `FilterConfig` without `disabled` | yes | `enabled` | where the entry is |

Request order is the HCM order of the running filters. Response order is that list reversed. Upstream filters (`HCMConfig.UpstreamFilters`) run for each upstream request after the router. Envoy doesn't allow them to be switched per route, so they are listed as they are.

## Model

These are additive changes. `krp dump -o json` stays `v1alpha1` (D-005: only removals or renames bump it).

```go
// Listener: state from the dynamic_listeners entry.
type Listener struct {
	// ... existing fields
	State ListenerState `json:"state"`
}

type ListenerState struct {
	Status      ListenerStatus `json:"status"`                // active | warming | draining | rejected
	Updating    bool           `json:"updating,omitempty"`    // active, with a newer version warming
	Draining    bool           `json:"draining,omitempty"`    // an older version is draining
	LastUpdated string         `json:"lastUpdated,omitempty"` // of the version shown, RFC 3339 as Envoy writes it
	Error       *ListenerError `json:"error,omitempty"`       // error_state: the last rejected update
}

type ListenerError struct {
	Details     string `json:"details"`
	LastAttempt string `json:"lastAttempt,omitempty"`
}
```

The listener shown is `active_state` if there is one, otherwise `warming_state`, then `draining_state`. A listener with only `error_state` is `rejected` and has no filter chains. An `error_state` next to an active listener keeps `Status: active` and sets `Error`, because Envoy keeps serving the old version. Static listeners stay out of scope, since kgateway doesn't use them.

```go
// Per-filter config at the two levels the parser doesn't read today.
type RouteConfig struct { /* ... */ TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` }
type VirtualHost struct { /* ... */ TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` }
type Route struct {
	// ... existing fields
	PerBackendFilterConfig bool            `json:"perBackendFilterConfig,omitempty"` // a weighted cluster sets typed_per_filter_config
	Chain                  *EffectiveChain `json:"effectiveChain,omitempty"`         // set by chain.Annotate
}

type EffectiveChain struct {
	Filters []ChainEntry `json:"filters"` // every HCM filter, in HCM order; Runs marks the ones that run
}

type ChainEntry struct {
	Name          string      `json:"name"`
	Type          string      `json:"type,omitempty"`          // HTTPFilter.Type
	Runs          bool        `json:"runs"`
	State         FilterState `json:"state"`                   // on | off | enabled | disabled
	Level         ConfigLevel `json:"level,omitempty"`         // route | virtualHost | routeConfig; empty for the HCM default
	PerFilterType string      `json:"perFilterType,omitempty"` // @type of the deciding per-filter config
	Policies      []PolicyRef `json:"policies,omitempty"`      // policies behind a per-route entry, when the mapper knows
}

func (c EffectiveChain) Request() []ChainEntry  // running filters, HCM order
func (c EffectiveChain) Response() []ChainEntry // Request reversed
```

`Filters` keeps the filters that don't run, because "why doesn't ext_authz run here?" is part of the same question. `Route.Chain` lives on the route, so every route in a narrowed snapshot has its chain without walking listeners again. A consumer that wants a map keys it however it likes. Route names can repeat across filter chains that share a route config, so xdsview doesn't key by name.

## Packages and API

```go
// pkg/envoy/chain — stdlib + model + origin.
// Effective resolves one route's chain (table above). Pure. The route-config
// level comes from hcm.RouteConfig.
func Effective(hcm model.HCMConfig, vh model.VirtualHost, r model.Route) model.EffectiveChain

// Annotate returns a copy of snap with Route.Chain set on every route, the way
// origin.Annotate copies. m may be nil; when set, it attributes policies (below).
func Annotate(snap *model.EnvoySnapshot, m origin.Mapper) *model.EnvoySnapshot
```

`chain` is a package of its own, not a `filter.Filter` option. Filter narrows a snapshot, and the chain is a per-route answer. Snellius needs the chain without narrowing, and kgwctl narrows first, then annotates.

**Policy attribution.** `Route.Policies` (Phase 5) lists `{Policy, Field}`, for example `{TrafficPolicy team-a/auth, extAuth}`. A new optional interface in `origin` says which entries a policy field explains:

```go
// PolicyFilters is implemented by mappers that know which filter a policy field configures.
type PolicyFilters interface {
	PolicyFilter(p model.PolicyRef, f model.HTTPFilter) bool
}
```

`origin.Kgateway` implements it with a small field-to-kind table taken from the corpus:

| Field | Filter kind |
|---|---|
| `extAuth` | `ext_authz` |
| `extProc` | `ext_proc` |
| `rateLimit.global` | `ratelimit` |
| `cors` | `cors` |
| `transformation` | kgateway's transformation filter |

The kind comes from `FilterOrigin.Kind`, or from the composite delegate type. `Chain` asks each mapper in turn. A vendor plugin adds its own fields (P-002). A policy is attached only to entries with a per-route `Level`. The interface is optional, so existing `Mapper` implementations still compile.

```go
// pkg/envoy/render — stdlib + model only (checked by the boundary test).
type Options struct {
	Header        string                         // first line, supplied by the caller (gateway, pod, capture time, file)
	ResponseOrder bool                           // also print each route's response order
	AllFilters    bool                           // list filters that don't run inline; default: one "not run:" line
	Style         func(c Class, s string) string // nil: plain text
	Expand        func(ref FilterRef) any        // nil: no inline config (TUI hook)
	Cursor        *FilterRef                     // TUI cursor; marked through Style(ClassCursor, …)
}

func Text(w io.Writer, snap *model.EnvoySnapshot, opts Options) error
```

`Class` is a small set of semantic roles: listener, vhost, route, filter, filter-off, origin, backend, warning, cursor. `internal/renderer` maps them to lipgloss styles and the TUI drives `Cursor` and `Expand`, so the tree's structure lives in one place. `FilterRef` moves from `internal/renderer` to `render`. If `snap` has no chains, `Text` computes them with `chain.Effective` and no mapper. Origins print when the snapshot is annotated.

Text shape (plain, LF-terminated, no trailing spaces, input order everywhere):

```
Gateway team-a/team-a · pod team-a-86d49b69c4-m86x6 · 2026-11-20 19:40:12 UTC
listener~80 [::]:80 · active · Gateway team-a/team-a listener http
└─ vhost listener~80~orders_example [orders.example] · listener http
   └─ route PathPrefix /orders · HTTPRoute team-a/orders · rule 0 · match 0
      filters (request order):
        1 io.solo.transformation                   on
        2 ext_auth/team-a/auth                     enabled by route · TrafficPolicy team-a/auth (extAuth)
        3 envoy.filters.http.router                on
      not run: ratelimit/team-a/rl (off), envoy.filters.http.cors (off)
      upstream filters: envoy.filters.http.upstream_codec
      request headers: + x-a: 1
      backend: kube_team-a_orders_8080
listener~8443 [::]:8443 · warming · error: <details> (2026-11-20T19:39:58Z)
```

The listener line shows the state and any error. A listener that isn't active shows its chains if Envoy has them, and never prints an empty tree without its state. The `krp` text output adopts this shape: the bordered listener panel goes and colour stays. The interactive TUI keeps cursor and expand.

## Boundary test

`core` gains `chain` and `render`. A second assertion covers kgwctl's need: no `github.com/charmbracelet/...` module, and nothing outside the standard library and `pkg/envoy` except through `source/kube`, appears in `go list -deps` of `render` or `chain`.

## Testing

- **chain:** a table over the semantics above, with synthetic snapshots for every row, the three levels and their precedence, the weighted-cluster flag, and an HCM without a route config. Corpus checks with literal expectations:
  - `03-multi-policy` on v2.4.7 and v2.5.0: ext_auth enabled by route through `FilterConfig`, ratelimit and cors enabled by route, `global_disable/ext_auth` off, router last, response order reversed.
  - `00-simple`: everything `on`, nothing per route.
  
  Policy attribution: `03` attributes `extAuth`, `rateLimit.global` and `cors` to their entries. A field without a mapping attributes nothing.
- **parser:** listener state from synthetic `dynamic_listeners`: warming only, active + warming, draining, error only, active + error. Per-filter config on vhost and route config. Existing goldens change only by additions; verify with jq as in Phase 5.
- **render:** golden `.txt` per corpus dump in `testdata/golden/` (`-update` flag, as for JSON), plus `ResponseOrder`, `AllFilters`, `Header` and non-active listener cases. Determinism: render twice, compare bytes. A test asserts no `\x1b` and no trailing whitespace.
- **krp:** existing renderer and TUI tests are kept where the information is the same and updated where the layout changes (listed in the sub-phase record). Interactive cursor and expand tests stay green.
- **Boundary test** as above. It fails if `render` imports lipgloss (checked by mutation).

## Compatibility

- Library: additive (`model` fields, new packages, optional interface). `internal/renderer`'s exported API is internal.
- `krp dump` text output changes layout (above). The flags stay the same.
- `krp dump -o json`: additive fields (`state`, `effectiveChain`, `typedPerFilterConfig` on vhost and route config, `perBackendFilterConfig`). Goldens are regenerated, and the diff contains additions only.
- Tag `v0.4.0`.

## Risks

- **A wrong chain is worse than none.** The semantics table is checked against the Envoy docs, not only against kgateway's habits. The Codex review is asked to check it. A live check on kind confirms one enabled-by-route filter, by sending a request that ext_authz must deny.
- **Plain text becomes a contract.** Golden `.txt` files make layout changes visible in review. Consumers that need structure use the model, not the text.
- **TUI regression** from moving the tree into `render`. Mitigated by keeping the interactive tests and doing one manual TUI run, recorded in the sub-phase record.

## Open questions for the owner

1. Default to one "not run:" line per route, with `AllFilters` listing those filters inline? kgwctl's sketch lists only running filters. Proposed: yes.
2. Drop the bordered listener panel from `krp` output in favour of the shared shape? Proposed: yes, since one tree means no drift.

## Done when

- [ ] `chain.Effective` and `chain.Annotate`, listener state and the vhost/route-config per-filter config exist, tested as above.
- [ ] `render.Text` is stdlib-only, with golden `.txt` files. `krp` and its TUI render through it.
- [ ] The boundary test covers `chain` and `render`, plus the no-charmbracelet assertion.
- [ ] README documents the new packages and the semantics table.
- [ ] Cross-model phase review triaged. `v0.4.0` tagged, the program file updated (E11 available, E4 note), and #27 closed with a notice to kgwctl and Snellius.
