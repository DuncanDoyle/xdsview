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
- Resolving per-backend config in a traffic split (`weighted_clusters[].typed_per_filter_config`). Envoy picks a backend per request. The model keeps the raw per-backend config, and the chain is marked conditional.
- What happens inside a matcher or composite filter (`ExtensionWithMatcher`). The filter is shown as running, and its delegate type is shown when Phase 5 resolved it.
- Whether a filter does anything on a given request (for example, a transformation with no rule that matches). The chain says which filters run, not what they change.
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
| false | `FilterConfig{config: …}` or a filter-specific config | yes | `on` | where the entry is (config overridden) |
| true | none | no | `off` | — |
| true | `FilterConfig{disabled: true}` | no | `off` | where the entry is |
| true | `FilterConfig{config: …}` (kgateway sends `config: {}`) or a filter-specific config | yes | `enabled` | where the entry is |

Envoy rejects a `FilterConfig` that has neither `disabled: true` nor `config` ([`per_filter_config.cc`](https://github.com/envoyproxy/envoy/blob/c7c57d8f48/source/common/router/per_filter_config.cc#L82-L122)), so an accepted dump never has one. If xdsview sees one anyway, the entry is `State: unknown`, and that is tested. Precedence follows [`config_impl.cc`](https://github.com/envoyproxy/envoy/blob/c7c57d8f48/source/common/router/config_impl.cc#L1438-L1443).

**Weighted clusters.** A selected backend's `typed_per_filter_config` overrides the route's ([`weighted_cluster_specifier.cc`](https://github.com/envoyproxy/envoy/blob/c7c57d8f48/source/common/router/weighted_cluster_specifier.cc#L235-L245)). When any backend of the route sets one, the chain is marked `Conditional`. The text then says "baseline before backend selection" and names the backends that override.

**Request and response.** Request order is the HCM order of the running filters that handle requests. Envoy runs response handlers in reverse, so response order is the reverse of the running filters that handle responses. Whether a filter type has a request or response side comes from a table in `chain`. The table lists each Envoy filter's decoder and encoder interfaces, checked against the Envoy source at the version kgateway ships (`/server_info`), and is cited in the code. A type that isn't in the table is `unknown` on both sides. It stays in both lists, marked, and is never dropped.

**Upstream filters** (`HCMConfig.UpstreamFilters`) are the router's configured upstream chain. Envoy can replace it per cluster and give it route-aware config ([`upstream_request.cc`](https://github.com/envoyproxy/envoy/blob/c7c57d8f48/source/common/router/upstream_request.cc#L155-L184)), so the text labels the list "router upstream filters (as configured)", not as the effective upstream chain.

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

The listener shown is `active_state` if there is one, otherwise `warming_state`, then `draining_state`. A listener with only `error_state` is `rejected` and has no filter chains. An `error_state` next to an active listener keeps `Status: active` and sets `Error`, because Envoy keeps serving the old version. `Updating` and `Draining` mark the other versions next to an active one, and the text shows them. Envoy can also report an LDS-wide error with no listener name ([`listener_manager_impl.cc`](https://github.com/envoyproxy/envoy/blob/c7c57d8f48/source/common/listener_manager/listener_manager_impl.cc#L555-L599)). xdsview keeps it as a `rejected` listener with an empty `Name`, and the text shows it as "LDS error". Static listeners stay out of scope, since kgateway doesn't use them.

```go
// Per-filter config at the levels the parser doesn't read today.
type RouteConfig struct { /* ... */ TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` }
type VirtualHost struct { /* ... */ TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` }
type WeightedCluster struct { /* ... */ TypedPerFilterConfig map[string]any `json:"typedPerFilterConfig,omitempty"` }
type Route struct {
	// ... existing fields
	Chain *EffectiveChain `json:"effectiveChain,omitempty"` // set by chain.Annotate
}

type EffectiveChain struct {
	Filters     []ChainEntry `json:"filters"`               // every HCM filter, in HCM order; Runs marks the ones that run
	Conditional bool         `json:"conditional,omitempty"` // a backend's per-filter config may override (weighted clusters)
}

type ChainEntry struct {
	Name          string         `json:"name"`
	Type          string         `json:"type,omitempty"`          // HTTPFilter.Type
	Runs          bool           `json:"runs"`
	State         FilterState    `json:"state"`                   // on | off | enabled | disabled | unknown
	Level         ConfigLevel    `json:"level,omitempty"`         // route | virtualHost | routeConfig; empty for the HCM default
	PerFilterType string         `json:"perFilterType,omitempty"` // @type of the deciding per-filter config
	Request       Side           `json:"request"`                 // yes | no | unknown: has a request (decoder) side
	Response      Side           `json:"response"`                // yes | no | unknown: has a response (encoder) side
	Contributions []Contribution `json:"contributions,omitempty"` // what configured this entry, when the mapper knows
}

// Contribution is one object behind a chain entry: the extension that defines
// the filter instance, or a policy field that configures it on this route.
type Contribution struct {
	Kind  ContributionKind `json:"kind"`            // extension | policy
	Ref   Ref              `json:"ref"`
	Field string           `json:"field,omitempty"` // policy field, e.g. "extAuth"
}

func (c EffectiveChain) RequestOrder() []ChainEntry  // Runs && Request != no, HCM order
func (c EffectiveChain) ResponseOrder() []ChainEntry // Runs && Response != no, reversed
```

`Filters` keeps the filters that don't run, because "why doesn't ext_authz run here?" is part of the same question. `Route.Chain` lives on the route inside its HCM. The parser clones a route config per HCM, so the same route name under two filter chains gets two chains. Every route in a narrowed snapshot has its chain without walking listeners again, and xdsview doesn't key chains by route name.

`Contributions` lists every contribution it can find, in a stable order. The dump says which policies were merged into a route, but not whether a policy was attached directly or inherited, for example from a Gateway. The two are not distinguished. That would need the Kubernetes objects, which belong to the consumer (D-002).

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

The kind comes from `FilterOrigin.Kind`, or from the composite delegate type. `Chain` asks each mapper in turn. A vendor plugin adds its own fields (P-002). A policy contribution is attached only to entries with a per-route `Level`. The extension contribution comes from `FilterOrigin.Extension`. The interface is optional, so existing `Mapper` implementations still compile.

**Narrowing (`filter.Filter`).** Two changes, so that the chain shown is the right route's:

1. When a route carries an `Origin` (the snapshot was annotated), `Filter` matches on it.
   - If the origin is `Exact`, or `Inferred` with one candidate, `Filter` compares that origin's HTTPRoute and rule.
   - If the origin is `Ambiguous`, the route never matches. `filter.Ambiguous(snap, opts) []string` returns the names of ambiguous routes whose candidates include the requested HTTPRoute, so the consumer can report them.
   - `RouteAnnotation` gains `NarrowedByKnown bool` (`json:"narrowedByKnown,omitempty"`). It is set when the route name alone reads several ways and `Known` left one candidate. A route still served for a deleted HTTPRoute can resolve to a live one that way. `Filter` still matches such a route, and the text marks its origin "(inferred from cluster routes)", so the consumer can decide.
   - Without an origin, matching stays as it is: by `route_source` metadata, otherwise by name. Today's name match can select HTTPRoute `orders` in `team-a` and `orders-team` in `a` alike. Both encode `httproute-orders-team-a-`.
2. Listeners that aren't active, and listeners with an `Error`, survive narrowing with their state, even with no matching route. They may explain why a route is missing.

```go
// pkg/envoy/render — stdlib + pkg/envoy core only (checked by the boundary test).
type Options struct {
	Header        string // first line, supplied by the caller (gateway, pod, capture time, file)
	ResponseOrder bool   // also print each route's response order
	AllFilters    bool   // list filters that don't run inline; default: one "not run:" line

	// Generic hooks for styled or interactive callers; nil gives plain text.
	Style func(c Class, ref *FilterRef, s string) string // ref is set on filter rows
	Extra func(ref FilterRef) []string                   // lines printed under a filter row
}

func Text(w io.Writer, snap *model.EnvoySnapshot, opts Options) error
```

`Class` is a small set of semantic roles: listener, vhost, route, filter, filter-off, origin, backend and warning. `FilterRef` (listener, filter chain, vhost, route and filter indexes) moves from `internal/renderer` to `render`. `internal/renderer` maps the classes to lipgloss styles. The TUI uses the same two hooks: `Style` highlights its cursor row, and `Extra` prints expanded config. So the tree's structure lives in one place, and no cursor or expansion concept enters the exported API. The TUI renders with `AllFilters: true`, so every filter it can select is a visible row.

If `snap` has no chains, `Text` computes them with `chain.Effective` and no mapper, so `render` imports `chain` and `origin`, both stdlib-only core. Origins print when the snapshot is annotated. A `Conditional` chain prints "baseline before backend selection" and names the backends that override it. Side `unknown` prints as `?` in response order.

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
      router upstream filters (as configured): envoy.filters.http.upstream_codec
      request headers: + x-a: 1
      backend: kube_team-a_orders_8080
listener~8443 [::]:8443 · warming · error: <details> (2026-11-20T19:39:58Z)
```

The listener line shows the state and any error. A listener that isn't active shows its chains if Envoy has them, and never prints an empty tree without its state. The `krp` text output adopts this shape: the bordered listener panel goes and colour stays. The interactive TUI keeps cursor and expand.

## Boundary test

`core` gains `chain` and `render`, so their dependencies are only the standard library and the `pkg/envoy` core. That rule already excludes `source/kube`, client-go and charmbracelet. An explicit second assertion covers kgwctl's check: no `github.com/charmbracelet/` path appears in `go list -deps` of `render`.

## Testing

- **chain:** a table over the semantics above, with synthetic snapshots for every row, the three levels and their precedence, the weighted-cluster flag, and an HCM without a route config. Corpus checks with literal expectations:
  - `03-multi-policy` on v2.4.7 and v2.5.0: ext_auth enabled by route through `FilterConfig`, ratelimit and cors enabled by route, `global_disable/ext_auth` off, router last, response order reversed.
  - `00-simple`: everything `on`, nothing per route.
  
  Policy attribution: `03` attributes `extAuth`, `rateLimit.global` and `cors` to their entries, and the ext_auth entry also gets its GatewayExtension. A field without a mapping attributes nothing.

  The remaining chain cases:
  - **Sides:** a known decoder-only filter (router, ext_authz) is absent from response order. A type outside the table is `unknown` in both orders.
  - **Invalid `FilterConfig`:** an entry with neither `disabled` nor `config` gives `unknown`.
  - **Conditional:** a weighted backend with per-filter config sets `Conditional`.
- **filter:**
  - A colliding pair (`orders`/`team-a` and `orders-team`/`a`) under `Known` selects only the requested route. Without `Known`, both are ambiguous, nothing matches, and `Ambiguous` names them.
  - Un-annotated snapshots keep today's results (existing tests).
  - A warming listener and a rejected listener survive narrowing.
- **parser:**
  - Listener state from synthetic `dynamic_listeners`: warming only, active + warming, draining, error only, active + error, and an unnamed LDS error.
  - Per-filter config on vhost, route config and weighted cluster, asserted on the joined snapshot after the per-HCM RDS clone, not only on the raw parse.
  - Existing goldens change only by additions. Verify with jq, as in Phase 5.
- **krp `-o json`:** goldens include `effectiveChain` (the CLI annotates before printing JSON as well as text).
- **render:** golden `.txt` per corpus dump in `testdata/golden/` (`-update` flag, as for JSON), plus `ResponseOrder`, `AllFilters`, `Header` and non-active listener cases. Determinism: render twice, compare bytes. A test asserts no `\x1b` and no trailing whitespace.
- **krp:**
  - Existing renderer and TUI tests are kept where the information is the same, and updated where the layout changes (listed in the sub-phase record).
  - A TUI navigation test presses through an enabled, disabled, enabled sequence, with one filter enabled by a vhost override. It checks the visible selected row and the expanded config at each step.
- **Boundary test** as above. It fails if `render` imports lipgloss (checked by mutation).

## Compatibility

- Library: additive (`model` fields, new packages, optional interface). `internal/renderer`'s exported API is internal.
- `krp dump` text output changes layout (above). The flags stay the same.
- `krp dump -o json`: additive fields (`state`, `effectiveChain`, `typedPerFilterConfig` on vhost, route config and weighted cluster). Goldens are regenerated, and the diff contains additions only.
- `filter.Filter`: results change only for annotated snapshots (origin-based matching, with ambiguous routes excluded) and for non-active or failed listeners, which now survive narrowing. Un-annotated results are unchanged.
- Tag `v0.4.0`.

## Risks

- **A wrong chain is worse than none.** The semantics table and the side table are checked against Envoy source, not only against kgateway's habits. A live check on kind confirms one enabled-by-route filter. Scenario `03`'s auth service has no endpoints, so a denial alone proves nothing. The oracle is Envoy's ext_authz stats: the HCM's `ext_authz` error or denied counter grows for a request to the enabled route, and doesn't grow for a request to a control route without it.
- **Phase 7 comparison.** Phase 7 compares intended and accepted config. It must treat `effectiveChain`, origins and listener lifecycle (`State`) as derived or runtime data, not as config. The raw per-backend config is kept for this reason (not reduced to one flag).
- **Plain text becomes a contract.** Golden `.txt` files make layout changes visible in review. Consumers that need structure use the model, not the text.
- **TUI regression** from moving the tree into `render`. Mitigated by keeping the interactive tests and doing one manual TUI run, recorded in the sub-phase record.

## Owner decisions (2026-11-21)

The owner approved the design on 2026-11-21 and answered both open questions:

1. The default output has one "not run:" line per route, and `AllFilters` lists those filters inline: **yes**.
2. `krp` output drops the bordered listener panel and uses the shared shape: **yes**.

## Done when

- [ ] `chain.Effective` and `chain.Annotate`, listener state and the vhost/route-config per-filter config exist, tested as above.
- [ ] `render.Text` is stdlib-only, with golden `.txt` files. `krp` and its TUI render through it.
- [ ] The boundary test covers `chain` and `render`, plus the no-charmbracelet assertion.
- [ ] README documents the new packages and the semantics table.
- [ ] Cross-model phase review triaged. `v0.4.0` tagged, the program file updated (E11 available, E4 note), and #27 closed with a notice to kgwctl and Snellius.
