# Phase 5 — Data-Plane Accuracy and Mapping: Design

**Base:** `badf0ab`
**Roadmap:** `docs/roadmap.md` § Phase 5
**Program:** edge E5 (Envoy name → Gateway API ref), decisions P-001 and P-002; E4 needs #25

## Goal

1. **Correct cancellation.** Cancelling a kube-source call stops the port-forward, including one that is still connecting (#25). This unblocks the kgateway-ui backend (E4).
2. **Faithful model.** The model captures what Envoy runs, including the parts krp drops today:
   - `prefix_rewrite` (#11)
   - `weighted_clusters` (#13)
   - router `upstream_http_filters` (#15)
   - each HTTP filter's type
3. **Mapping back to Gateway API (E5, P-001).** A library answers "which Gateway API object produced this Envoy object?" for listeners, virtual hosts, routes, clusters, HTTP filters and policies. The UI's `computed.EnvoyNamer` and kgwctl call it. A consumer can swap in its own implementation.
4. **Plugin seam (P-002).** Vendor policy kinds plug in from outside the core. The core contains no vendor or edition names.
5. **Rule names (#16).** `--rule` accepts a rule name. The HTTPRoute rule `name` field is in the standard channel in Gateway API v1.6.1 and v1.7.0, and kgateway appends it to the route name.
6. **Corpus (#17).** Real dumps for multi-policy, multi-rule, multi-listener, traffic split and named rules, plus one recording with route-source metadata enabled.

## Non-goals

- Intended vs accepted config (Phase 6).
- Reading Kubernetes objects. The mapping works from the dump alone, and the caller can pass in object refs it already knows (§ Ambiguity).
- HTTPRoute delegation (parent/child routes). Its route names aren't covered by the corpus, and a child route maps to Unknown or Inferred like any other route.
- TCP/TLS routes, ListenerSets and GRPCRoute. Their names are recognised where cheap, but they aren't verified with a corpus.
- Changing the CLI's text layout beyond adding origin lines.

## What kgateway emits (v2.4.7 and v2.5.0, identical)

Sources:
- the v2.4.7 and v2.5.0 tags, read in the kgateway source
- krp's OSS corpus

| Envoy object | Name | Built by |
|---|---|---|
| Listener | `listener~<port>` | `GenerateListenerNameFromPort` |
| Filter chain / route config | HTTP: `listener~<port>`; HTTPS: `<sectionName>`, or `<lsNs>/<lsName>/<sectionName>` for a ListenerSet | `query.GenerateRouteKey` |
| Virtual host | `SanitizeForEnvoy(<chain>~<hostname>)`: `.` becomes `_`, and a route with no hostname gets `*` | `makeVhostName` |
| Route | `<vhost>-route-<i>-<kind>-<name>-<ns>-<ruleIdx>-<matchIdx>[-<ruleName>]-matcher-<matchIdx>` | `UniqueRouteName`, `initRoutes` |
| Cluster | `kube_<ns>_<svc>_<port>`; other backends `<prefix>_<ns>_<name>[_<extra>]_<port>` | `buildClusterName` |
| Policy HTTP filter | `ext_auth/<ns>/<name>`, `ext_proc/<ns>/<name>`, `ratelimit/<ns>/<name>`, `jwt/<ns>/<name>`, where `<ns>/<name>` is the **GatewayExtension** | trafficpolicy plugin |
| Helper filter | `global_disable/<kind>`: a set_metadata filter, not a policy | trafficpolicy plugin |

Three facts shape the design:

1. **`<i>` is not the rule index.** It's the route's position in the virtual host after precedence sorting. The rule index is the first number after the namespace, and the second is the match index. krp's filter docs call that second number the "backend" index, which is wrong (fixed in 5.3).
2. **Route names are ambiguous to reverse.** Names, namespaces and rule names all allow `-`. For example, `httproute-a-b-c-0-0` is either HTTPRoute `a-b` in `c` or `a` in `b-c`. Forward matching, which finds the routes of a known HTTPRoute, is unambiguous apart from rare collisions. Virtual hosts and `kube_` clusters reverse cleanly, because `~` and `_` can't appear in the names they separate.
3. **Metadata beats names.**
   - **`merge.<Kind>.<group>`** route metadata gives policy provenance as `{<policyField>: ["<group>/<Kind>/<ns>/<name>"]}`. It's present on every policy scenario in both corpora; the enterprise corpus uses its own Kind.
   - **`dev.kgateway.route_source`** route metadata gives `{kind, group, name, namespace, rule, match}`. It's exact, but off by default (Helm `enableRouteSourceMetadata`).
   - HTTP filters carry no source metadata.

kgateway has no reverse parser. Its forward builders are mostly unexported, so a CLI inside kgateway still needs this mapping, or new exports upstream (P-001's swap point).

## Package layout

```
pkg/envoy/model      + Route.WeightedClusters, Route.Rewrite.Prefix, HTTPFilter.Type, Router upstream filters
pkg/envoy/parser     parses the new fields
pkg/envoy/origin     NEW: mapping interface, ref types, kgateway OSS implementation, composition
pkg/envoy/filter     uses origin for route selection; rule names
pkg/envoy/source/kube  #25 fix
```

`origin` is standard-library only, like the rest of the core. The boundary test covers it.

The origin **value types** (`Ref`, `RouteOrigin`, `Certainty` and the other `*Origin` types) live in `model`, so the snapshot can carry them without an import cycle. `origin` holds the `Mapper` interface, the implementations and `Annotate`, and imports `model`.

## API

```go
package model // value types; origin uses them

// Ref names a Kubernetes object. Group is empty for the core group.
type Ref struct{ Group, Kind, Namespace, Name string }

// RouteOrigin is the HTTPRoute rule and match an Envoy route came from.
type RouteOrigin struct {
	Route      Ref
	RuleIndex  int    // -1 when unknown
	RuleName   string // empty when the rule has no name
	MatchIndex int    // -1 when unknown
}

// Certainty says how an origin was derived.
type Certainty int

const (
	Unknown   Certainty = iota // not recognised
	Ambiguous                  // several candidates; see Candidates
	Inferred                   // parsed from a name
	Exact                      // read from metadata
)

package origin

// Mapper maps Envoy objects to the Gateway API objects that produced them.
// Every method is pure and safe for concurrent use.
type Mapper interface {
	Listener(l model.Listener) (ListenerOrigin, Certainty)            // port; Gateway when known
	RouteConfig(name string) (ListenerOrigin, Certainty)              // route config / filter chain name: port or section; Gateway when known
	VirtualHost(vh model.VirtualHost) (VirtualHostOrigin, Certainty)  // listener section, hostname
	Route(r model.Route) ([]RouteOrigin, Certainty)                   // >1 only when Ambiguous
	Cluster(name string) (BackendOrigin, Certainty)                   // backend ref + port
	HTTPFilter(f model.HTTPFilter) (FilterOrigin, Certainty)          // kind, extension ref, helper flag
	Policies(r model.Route) []PolicyRef                               // from merge.* metadata
}

// Kgateway returns the mapper for kgateway OSS, verified on v2.4.7 and v2.5.0.
func Kgateway(opts KgatewayOptions) Mapper

type KgatewayOptions struct {
	Gateway Ref   // the proxy's Gateway, when the caller knows it (listener and vhost origins)
	Known   []Ref // HTTPRoutes the caller knows; used to resolve ambiguous route names
}

// Chain tries each mapper in order and returns the first result better than Unknown.
func Chain(mappers ...Mapper) Mapper

// Annotate returns a copy of snap with origins filled in (see § Model).
func Annotate(snap *model.EnvoySnapshot, m Mapper) *model.EnvoySnapshot
```

**Route resolution order:** `route_source` metadata (Exact), then name parsing. When the name splits more than one way, `Known` picks the one that exists (Inferred); otherwise the result is Ambiguous with every split. Forward selection in `filter` never needs this.

**HTTP filter kind** comes from `typed_config.@type`, unwrapping `ExtensionWithMatcher`, not from the name. The name's `<ns>/<name>` suffix gives the GatewayExtension. `global_disable/*` is reported as a helper.

**UI adapter.** The UI's `EnvoyNamer.Object(envoyName string) (objects.Ref, bool)` works on bare names: route config and cluster names from proxy stats. `origin.ObjectForName(m Mapper, name string) (Ref, bool)` covers that case. It only dispatches through the mapper, so a replacement mapper changes it too: it tries `m.Cluster`, then `m.RouteConfig`, and returns the first Inferred or Exact result, otherwise `false`. A route config name such as `listener~80` or `https-api` holds no Gateway identity. It maps to the Gateway in `KgatewayOptions.Gateway`, and is Unknown when that isn't set. The UI builds the mapper with the Gateway whose proxy it is reading. The UI keeps its own pinned rule set and calls it.

## Plugin seam (P-002)

The seam is the `Mapper` interface plus `Chain`. A vendor plugin is a `Mapper` that recognises its own filter names, metadata keys or kinds, and returns Unknown for everything else. The consumer composes them: `origin.Chain(vendorMapper, origin.Kgateway(opts))`.

The core is generic in two ways:
- it reads any `merge.<Kind>.<group>` key, so a vendor policy Kind shows up as a `PolicyRef` without a plugin;
- it classifies filters by Envoy `@type`, not by vendor names.

The test proves what a consumer sees. A test-only mapper in `origin_test` recognises one made-up filter name in a small synthetic dump, a fixture that doesn't depend on `testdata/enterprise/` (removed before E6). After `Annotate(snap, Chain(test, Kgateway(...)))`, that filter carries the plugin's origin, and the OSS filters in the same dump carry the OSS origins. The CLI golden test also renders one plugin origin, through a test-only hook in `cmd/krp`.

## Model

These changes are additive. JSON stays `v1alpha1` (D-005), and the golden diffs show only new fields.

| Field | Source | Issue |
|---|---|---|
| `Route.Rewrite.Prefix` | `route.prefix_rewrite` | #11 |
| `Route.WeightedClusters []WeightedCluster{Name, Weight}` | `route.weighted_clusters.clusters` | #13 |
| `HCMConfig.UpstreamFilters []HTTPFilter` | router `typed_config.upstream_http_filters` | #15 |
| `HTTPFilter.Type` | `typed_config.@type`, unwrapped from `ExtensionWithMatcher` | mapping |
| `Route.Origin`, `Route.Policies`, `VirtualHost.Origin`, `HTTPFilter.Origin`, `Listener.Origin` | set only by `origin.Annotate`; omitted from JSON when empty | E5 |

`parser.Parse` doesn't annotate. The CLI does: `krp dump` annotates by default, and the origins show in text and JSON. The kube source passes the Gateway it connected to as `KgatewayOptions.Gateway`.

## `--rule` by name (#16)

`--rule` takes an integer index or a rule name. `filter.Options` gets `RuleName string`, and `RuleIndex` stays. A route matches a name when its origin's `RuleName` equals it (from `route_source`), or when its name **ends with** `httproute-<name>-<ns>-<ruleIdx>-<matchIdx>-<ruleName>-matcher-<matchIdx>`, with the same match index in both places. Anchoring to the end matters, because rule names may contain `-matcher-`. Rule `a` must select neither `a-b` nor `a-matcher-b`.

## #25 cancellation

The fix makes the port-forward's connect honour `ctx`. `ConfigDump` with a cancelled ctx must leave no goroutine or connection behind once the call returns. The fix may take a little longer than an immediate return; the bound is set in the plan.

The boundary test uses a local server that accepts the TCP connection but never answers the upgrade, then cancels. The test asserts that the server sees the connection close, and that the forwarder goroutine has exited (`ForwardPorts` returned). The fake-tunnel seam from Phase 4 stays for the other cases.

The approach (a context-aware upgrade request versus a dial timeout) is the plan's choice. The design only fixes the observable behaviour.

## Corpus

| Scenario | Content | Exercises |
|---|---|---|
| 03-multi-policy | One HTTPRoute, CORS + ext auth + rate limit TrafficPolicies | `merge.*` with several policies, several filters per route |
| 04-multi-rule | One HTTPRoute, 3 rules (one with 2 matches) to 2 Services | rule and match indices; `<i>` ≠ rule index |
| 05-multi-listener | HTTP + HTTPS listeners, one HTTPRoute attached to both | pruning across listeners, HTTPS vhost names |
| 07-traffic-split | One rule, two weighted backendRefs | `weighted_clusters` (#13) |
| 08-named-rules | Rules named `a`, `a-b` and `a-matcher-b` | `--rule <name>`, the prefix trap |
| 04 + `route_source` | 04 recorded again with `enableRouteSourceMetadata=true` | Exact origins; Inferred and Exact must agree |

Each scenario is recorded on v2.4.7 and v2.5.0 with `hack/record-dumps.sh`, which gains a Helm values option for the `route_source` variant. `prefix_rewrite` (#11) and `upstream_http_filters` (#15) aren't emitted by kgateway OSS, so they're tested with synthetic parser fixtures. The enterprise 06 dump has real upstream filters (`envoy.filters.http.late_ext_proc`, then `envoy.filters.http.upstream_codec`), and gets explicit parse and render expectations while it's in the repo.

## Testing

| Boundary | Test |
|---|---|
| Parser | New fields from synthetic fixtures and from the 07 dump |
| `origin` on real dumps | Per scenario and version, a literal table: every route's origin, every policy filter's extension ref and kind, every `merge.*` policy ref. Written from the manifests, not computed. |
| Inferred vs Exact | On the `route_source` recordings, for every route: without `Known`, the name-parsed candidates contain the metadata origin. With `Known` taken from the scenario's manifests (a literal list in the test), the result is that one origin, Inferred. |
| Ambiguity | Synthetic route names with dashes in name and namespace: Ambiguous without `Known`, resolved with it |
| Seam | Test-only plugin on a synthetic dump, observed through `Annotate`, as in § Plugin seam |
| Filter | Phase 4 e2e tables, plus 04 per rule and 08 by name (`a` doesn't select `a-b`) |
| CLI | Golden files regenerated; the diff contains only additions |
| #25 | As in § #25 |
| Boundary | `origin` in the stdlib-only import test |

## Phase 6 boundary

Origins and certainty describe the mapping, not Envoy. They depend on the mapper, `Known`, `Gateway` and metadata. The Phase 6 comparison of intended and accepted config ignores the origin fields; they only label the differences it finds.

## Compatibility

- `filter.Options` and `Filter` keep their behaviour for existing callers. `RuleName` is a new field.
- The JSON changes are additive. The text output gains origin lines, and the text golden comparison is regenerated and reviewed.
- Release tag `v0.2.0`. The program file gets E5 as available, and E4 as unblocked for `source/kube`.

## Risks

| Risk | Mitigation |
|---|---|
| A kgateway version changes a name format | Per-version literal tables. A format break fails the test, it isn't skipped. `Kgateway` documents the verified versions. |
| `route_source` is experimental in kgateway (Helm comment: "subject to breaking changes") | It's only used when present, and the name parse stays the fallback. A shape change shows up in the Inferred = Exact test. |
| `merge.*` is not always written | kgateway's code suggests it's only written when policies merge, yet every single-policy scenario in the corpus has it. Policies are best-effort, and `PolicyRef` lists what the metadata says. |
| Origins wrong for ListenerSets and TCP/TLS | Returned as Unknown rather than guessed. Recognised later, when there's a corpus. |
| Scope: five sub-phases | #25 comes first and stands alone; mapping lands before the CLI uses it. |

## Open questions for the owner

1. **Ambiguous route names.** The mapper returns every candidate as Ambiguous rather than guessing. The UI adapter returns `false` for it. (Recommended; the alternative is picking the longest name.)
2. **Annotate by default in the CLI.** Recommended yes. The alternative is an `--origins` flag.

## Done when

- #11, #13, #15, #16, #17 and #25 are closed with commits.
- `origin` exists with the Kgateway mapper, `Chain`, `Annotate` and `ObjectForName`, verified on both versions.
- `krp dump` shows origins in text and JSON.
- The corpus has 03, 04, 05, 07 and 08 on both versions, plus the `route_source` variant.
- `v0.2.0` is tagged, the program file is updated, and kgwctl and kgateway-ui are notified.
