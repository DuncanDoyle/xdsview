# Phase 9 — Control-Plane-Agnostic Contract: design

**Status:** built; amended to match the implementation (2026-11-26); consumer confirmation pending
**Edge:** E15 | **Program:** P-010 (contract), P-011 rule 6 (snapshot-local IDs), P-012 rule 4 (version-keyed tables); the last two in program.md "Architecture guidelines" (`dbf7140`) | **Decisions:** D-019, D-020, D-021, D-022
**Release:** one tag at phase close, `v0.7.0`: the break plus core correctness (D-020)
**Inputs:** [approach consult](reviews/approach-codex.md), [kgwctl review](reviews/consumer-kgwctl.md), [Snellius review](reviews/consumer-snellius.md), [visibility-strategies consult](reviews/visibility-strategies-codex.md), [advisory review](reviews/phase9-advisory-fable.md), Phase 8 deferrals ([phase-8-complete.md](../phase-8/phase-8-complete.md)), E14 missing RDS ([xds-convergence.md §4.1](../../investigations/xds-convergence.md))

## Changes since consumer review (8d381f6)

kgwctl and Snellius reviewed this design at `8d381f6`. These changes are visible to a consumer:

1. Claim JSON nests the kind under `assertion` (`"assertion": {"kind": "routeSource", …}`), not at claim level.
2. An unknown assertion kind decodes as `provenance.Opaque`; `Compose` drops it.
3. `Index.Inputs(block)` lists every input of a block with a status (`covered`, `gap`, `bootstrap`), a gap reason and, for a route, its candidates.
4. `RouteSource` has `Incomplete` and `Exhaustive`. A route's candidate list is complete only when the module marks every top reading `Exhaustive`.
5. `Dependencies` is fail-closed: every input is enumerated, and each uncovered input is a gap.
6. `Dependencies.Objects` is `[]Dependent` (object plus claim kind). `Gap` has `Slot`, `Level` and `Cluster`.
7. Static (bootstrap) listeners and everything in them have input status `bootstrap`: never a gap. Route config a static listener fetches over RDS is not bootstrap: it gets ordinary claims, inputs and gaps, and `Select` can pick its routes.
8. The results of `Dependencies` and `Inputs` may change in a minor release until Snellius Phase 12 is designed. Their Go shapes may not.
9. Mirror clusters are inputs. `BackendSource.Mirror` names one, and `ResolveMirror` resolves it.
10. `Selection.Narrowed` lists only routes that are in `Routes`, `Ambiguous` or `Unresolved`.
11. `Narrowing` is a `provenance` type. `kgateway.Narrowing` is an alias.
12. There is no `Guess` and no route-name kind. `kgateway.Names()` answers unknown for route config and listener names.
13. `provenance.Notes(ix)` is a `render.Options.Trailer` hook (lines after a route's filters), not a `Notes` hook.
14. A `Module` must not modify the snapshot. The per-module copy is a safeguard, not a promise.
15. The default filter chain is the last entry of `FilterChains` with `Default: true`. There is no `DefaultFilterChain` field.
16. Static listeners get IDs after the dynamic ones.
17. `PolicyRecord.Level` must match its subject. `ListenerSource` may also sit on a virtual host.
18. krp JSON: all warnings are in the envelope's `warnings`; `selection` has `query`; a narrowed entry embeds its kept and dropped claims and may be `"selected": true` (D-022).
19. `HeaderMatch.Regex`, `QueryParamMatch.Regex` and `Route.Cluster` stay in the model.
20. Raw `Metadata` is not an attribution input. A consumer with per-object visibility must not render it to a restricted viewer.
21. `Query.RuleIndex` is `*int` (nil: any rule); there is no `AllRules`. With `RuleName` set, both must match.
22. `Query.Known` narrows readings by the rules in §6: never an exact reading, never the last one. There is no `narrowed` certainty label.
23. `kgateway.Names()` takes no options. `NameAnswer` has no `Kind`: each candidate `Ref` carries its own. `CheckedMinors()` lists the checked kgateway minors.
24. `model.CompareID`, typed snapshot lookups (§2) and `SourcePath` on virtual hosts and routes.
25. Warning codes are exported constants in `model` (§2).
26. `Index.Claims`, `Index.Record` and `provenance.Records`. `Assertion` has `Kind()`.
27. Index order is subject first (`CompareID`), then module, then emission. Compose rule 3 checks more: subject kind, evidence objects, level, certainty cap, repeated `Ref`.
28. An HCM whose route config isn't joined has its HCM default chain under its use ID.
29. `EffectiveChain.Conditional` marks a chain a weighted backend's per-filter config may override.
30. `provenance.Labels` is a `render.Options.Labels` hook; `render.Text` prints a subject's warnings under its row.
31. Missing-RDS and ECDS-default verdicts need a complete capture, a fully read section and an active listener (§4 items 1 and 4).
32. krp JSON: `selection.unresolved` exists; `selection.query` has `route` (`model.Ref`), optional `ruleIndex` (integer) and `ruleName`; claims and links are projected onto the retained snapshot (§7).
33. `Input.Complete` is false when any listed reading, at any certainty, is `Incomplete`.
34. A route config use that joined no source is an input with reason `unresolved`. A route config source block holds the inputs of every use of it.
35. A route's direct response is a `backendSource` input, always a gap (`notCoverable`). This changes results only (§5 Stability).
35. `Select` counts readings as `Resolve` merges them. One reading at `ambiguous` certainty stays ambiguous.

## Goal

The core answers what Envoy runs, from Envoy data alone. Product modules add claims about where each piece came from. Consumers join the two on occurrence IDs. After this phase, contract changes are additive (P-010 rule 7).

## Requirements

| # | Requirement | Source |
|---|---|---|
| R1 | Execution (parse, effective chain, order, sides, run state, warnings) depends only on Envoy data. The core imports no module and no provenance package. | P-010 rule 1, scope 1 |
| R2 | Execution and provenance are separate results. Modules get isolated input. Independent parses with zero modules and with every supported composition give identical snapshots and execution results. | rule 2, scope 2 |
| R3 | Every snapshot occurrence has an ID: listener, filter chain, network filter, HCM, route config use, virtual host, route, weighted backend, HTTP filter slot. Results and claims are keyed by ID, not by name. | scope 4, Codex #4 |
| R4 | One claim per assertion, with its own certainty, evidence and module. A policy record and its filter link are separate claims. The link names the record and one slot. | rule 4, scope 3 |
| R5 | `merge.*` records at listener, route config and virtual host level, beside route level, each level-local. | rule 4, scope 5, D-019 |
| R6 | The evidence scope of any block is enumerable: the objects its claims rest on, plus whether any occurrence in it is unattributed. | scope 5 (Snellius) |
| R7 | Composition is explicit and deterministic. Conflicting claims of equal certainty become ambiguous. No first-wins. | rule 6 |
| R8 | Candidate routes come from the kgateway module. Selection and pruning are consumer calls. Every candidate is kept; narrowing is recorded. | scope 6 |
| R9 | A `NameMapper` maps a bare Envoy name to objects, with certainty, resource kind and ambiguity. | scope 7, E5, carry-over (ObjectForName certainty) |
| R10 | krp JSON `v1alpha2`. kgwctl can build its own payload from public types without embedding `model`. | scope 8, E15 |
| R11 | The core never invents execution. What it cannot establish stays in place with explicit uncertainty. | rule 3 |
| R12 | Core correctness: the list in §4. | P-010 "core correctness", E14 |
| R13 | Phase 8 deferred P1s #2, #4, #5 fixed in the kgateway module. | phase-8-complete.md |
| R14 | Version-keyed tables (sides, policy field to filter) name the Envoy or kgateway version they were checked against. A test checks membership against the corpus. Each entry cites its source (Envoy factory, kgateway field), re-read at every bump: the documented fallback for entry correctness. | P-012 rule 4, 6 |
| R15 | After the tag, contract changes are additive and name a consumer case. A downstream module needs only exported API: a test module in an external test package (`provenance_test`) implements `Module`. | rule 7, rule 8, P-002 |

## Non-goals

- Stable IDs across dumps. Occurrence IDs are snapshot-local (P-011 rule 6). A cross-dump join is the consumer's, by name, and reports ambiguity.
- The live convergence verdict (kgwctl#13) and any stats parsing (E14).
- Delegation paths (U10), exact filter links (U11), wider policy scopes (U1). Additive later, on a named case.
- `version_info` and `version_text` (Phase 8 review #6). They belong to E14's kgwctl join; added when it asks.
- A downstream product module. The seam must support one; none ships here.

## Design

### 1. Layers and packages

| Layer | Package | Contents | May import |
|---|---|---|---|
| core | `pkg/envoy/model` | snapshot types with IDs, `ID`, `CompareID`, `Certainty`, `Ref`, `Prune`, warning codes | stdlib |
| core | `pkg/envoy/parser` | config dump → `*model.Snapshot` | model |
| core | `pkg/envoy/chain` | `Effective`: effective chain per route; the sides table | model |
| core | `pkg/envoy/render` | text view of a snapshot and its chains; hooks keyed by `Row` | model |
| core | `pkg/envoy/source`, `source/kube` | dump sources, with `Capture()` on `Source` | `source` stdlib and model; `source/kube` adds client-go |
| contract | `pkg/envoy/provenance` | `Claim`, assertion kinds, `Module`, `Compose`, `Index`, `Resolve`, `NameMapper`, `Narrowing`, `Labels`, `Notes`, `Records` | model, chain, render |
| module | `pkg/adapter/kgateway` | claims, `merge.*` and `route_source` decoding, route selection, names | model, provenance |

`origin` and `filter` do not exist. Three tests guard the layers (`pkg/envoy/boundary_test.go`):
1. `model`, `parser`, `source`, `chain` and `render` import only the standard library and each other.
2. `model`, `parser`, `chain` and `render` do not reach `provenance` or `adapter/...`.
3. `provenance` and `adapter/kgateway` import only the standard library, the core and `provenance`.

A fourth test fails if anything imports the removed `origin` or `filter` packages.

### 2. Snapshot and occurrence IDs (R3, R11)

`parser.Parse(r io.Reader, opts parser.Options) (*model.Snapshot, error)`. Warnings are in the snapshot, typed and keyed by ID. Only three errors are fatal: a read error, invalid JSON, and trailing data after the document. Everything else is a warning.

The same bytes can come from a complete or a filtered dump. `model.Capture` says which: `CaptureUnverified` (`"unverified"`) or `CaptureComplete` (`"complete"`). `Parse` treats an empty `parser.Options.Capture` as unverified. `source.Source` has `Capture() model.Capture`: `Admin` and `kube` return `CaptureComplete` when they fetched `/config_dump` with no filter or mask; `File` returns unverified. `Admin` fails on a redirect instead of following it. `Snapshot.Capture` carries the value. Missing-RDS and ECDS defaults (§4) need `CaptureComplete`.

```go
// ID names one occurrence in one Snapshot. It is opaque and valid only in
// the snapshot that produced it (program P-011 rule 6).
type ID string

// Within reports whether id is block or lies inside it. An empty block contains nothing.
func (id ID) Within(block ID) bool

// CompareID compares segment by segment, numerically on a segment's index:
// "L2" sorts before "L10", and a block before what lies inside it.
func CompareID(a, b ID) int

type Snapshot struct {
	Capture      Capture
	Listeners    []Listener
	RouteConfigs []RouteConfigSource // every route config source, in section order; in a routes section, static before dynamic
	Coverage     Coverage            // §4 item 9
	Warnings     []Warning
}

type Warning struct {
	Subject ID     // empty for dump-wide warnings
	Code    string // one of the exported Warn* constants
	Message string
}
```

Warning codes are exported constants in `model`: `section-skipped`, `route-config-skipped`, `hcm-skipped`, `rds-ambiguous`, `rds-missing`, `ecds-ambiguous`, `ecds-skipped`, and on a composed index `claim-dropped` and `link-dropped`. An `hcm-skipped` warning names the filter chain, since a skipped HCM has no occurrence.

IDs are assigned once, in `parser.Parse`, as structural paths:

| Occurrence | ID |
|---|---|
| listener | `L0`; dynamic listeners first, then static ones, so a bootstrap change doesn't move dynamic IDs |
| filter chain | `L0/C1`; the default filter chain is `L0/Cd` |
| network filter | `L0/C1/N0`; an HCM has its network filter's ID |
| HTTP filter slot | `L0/C1/N0/F3` |
| router upstream filter slot | `L0/C1/N0/U0` |
| route config use | `L0/C1/N0/RC` |
| virtual host, route, weighted backend | `.../RC/V2`, `.../RC/V2/R5`, `.../R5/W1` |
| route config source | `RD<n>` (dynamic), `RS<n>` (static), `<HCM>/RCI` (inline) |

Consumers compare IDs and call `Within`; they never parse them.

**Listener versions.** A dynamic listener shows one version: `active` if present, else `warming`, else `draining`. With none, its status is `rejected`. An active listener also records whether a version is warming (`Updating`) or draining (`Draining`).

**Route config sources and uses.** A source (`RouteConfigSource`) is one route configuration as the dump holds it: ID, kind (`dynamic`, `static`, `inline`), name, `Reported`, and its route-configuration-level `Metadata` and `TypedPerFilterConfig`. A source carries no virtual hosts. Each HCM has one use (`RouteConfigUse`): ID, `Source` (empty unless found) and `State`: `found`, `missing`, `ambiguous` or `unknown`. `HCMConfig.RouteConfig` holds the use's copy of the source's content, with use-scoped IDs.

RDS names resolve against dynamic sources only. Two dynamic sources with one name are both kept; an HCM that references the name gets `ambiguous`, no join and an `rds-ambiguous` warning. A static entry with the same name and content as an inline config is the configuration Envoy reports for it: the inline source records the static ID as `Reported`, and the static entry stays a source, so the ID resolves. Each inline config pairs with the first static entry not yet paired; the comparison ignores `@type`. Any other static entry is a source with no use. Claims about route-configuration-level data (`merge.*`, per-filter config) are on the source; a use inherits them (§5).

A route config shared by two HCMs has one source (`RD3`) and two uses (`L0/C1/N0/RC`, `L1/C0/N0/RC`), each with its own virtual host and route IDs. Every virtual host and route from a source has `SourcePath` (`RD3/V2`, `RD3/V2/R5`), equal across the uses of a shared source, so a consumer can list it once. `SourcePath` is not an occurrence ID.

**Lookups.** `Snapshot` has typed lookups by ID, each returning the item and a bool: `Listener`, `FilterChain`, `HCM` (also for any ID inside the HCM), `VirtualHost`, `Route`, `WeightedCluster`, `Slot` (downstream or upstream) and `RouteConfigSource`.

**Prune.** `model.Prune(snap, keep func(ID) bool)` returns a copy that keeps the selected routes and their ancestors:
- A virtual host with no kept route is removed.
- Every filter chain of a retained listener stays; a chain with no kept route keeps its HCM with no virtual hosts.
- A listener with no kept route is removed unless it may explain a missing route: it is not active, it has a version warming or draining, or Envoy rejected its last update. An empty status counts as active.
- A source stays when a retained HCM uses it, with the static entry a kept source reports.
- A warning stays when it is dump-wide or its subject is retained.

IDs survive pruning, so any result computed on the full snapshot still joins. The copy shares maps and unpruned slices with its input; both are read-only once shared.

The model holds no provenance: no origins, policies or chains on listeners, filters, virtual hosts or routes.

### 3. Execution (R1, R11)

```go
// Effective resolves the chain of every route in snap, keyed by route ID. It
// reads snap only. An HCM whose route config isn't joined has its HCM default
// chain under its route config use ID.
func Effective(snap *model.Snapshot) map[model.ID]model.EffectiveChain

type Tri string // "yes", "no", "unknown"

type EffectiveChain struct {
	Filters     []ChainEntry // every HCM filter, in HCM order
	Conditional bool         // a weighted backend's per-filter config may override
}

type ChainEntry struct {
	Slot          model.ID
	Name, Type    string
	Runs          Tri         // unknown: rejected per-filter config, unresolved ECDS
	State         FilterState // on, off, enabled, disabled, unknown, unresolved
	Level         ConfigLevel // route, virtualHost, routeConfig; empty for the HCM default
	PerFilterType string
	Request       Tri
	Response      Tri
	Wrapper       string // "composite" or "extensionWithMatcher"; empty otherwise
	Conditional   bool   // the entry's matcher can pick a delegate or skip the filter
	ConfigSource  string // "inline", "ecds", "ecdsDefault"; empty when unresolved
}
```

`Conditional` has two meanings by level. On `EffectiveChain` it means a weighted backend may override per-filter config; the entries are the baseline before backend selection. On `ChainEntry` it means the entry's matcher decides.

Per-filter config is read at route, virtual host and route config level, most specific first. A `FilterConfig` with `disabled: true` wins over its config. One with no `config` and `disabled` unset or false is one Envoy rejects: `Runs` unknown, `State` unknown.

`RequestOrder` and `ResponseOrder` cover the downstream slots. They drop an entry whose `Runs` is `no` or whose side on that axis is `no`, and keep entries whose run state or side is unknown. `ResponseOrder` is in reverse HCM order. Text shows unknown entries in position with `?` and the reason, never under `not run:`.

The sides table stays in `chain` (modules never supply sides, rule 3). It names its Envoy version (`sidesEnvoyVersion`, `v1.39.6 (aacd0340)`). Two tests check it (R14):
1. `TestSides_CoverCorpus`: every HTTP and upstream filter type in the corpus and fixtures is in the table. There are no exclusions.
2. `TestVersionTables_MatchCorpus`: the table's Envoy version is the one the newest recorded dump runs (`server_info.json`). The same test checks the kgateway tables (§6).

### 4. Core correctness (R12)

Each item changes OSS output. Goldens change only through reviewed `-update` runs.

| # | Item | Behaviour |
|---|---|---|
| 1 | Missing RDS (E14) | An HCM with an RDS name and no matching dynamic source gets `missing` and an `rds-missing` warning only when all of these hold: the capture is complete; the `RoutesConfigDump` was read in full, with no skipped entry (static ones included) and no unreadable section; the listener is active. A static listener also needs an active dynamic listener, which shows Envoy's workers started. Otherwise the use is `unknown`. |
| 2 | Static listeners and route configs | `static_listeners` and `static_route_configs` are parsed (§2). `Listener.Static` marks a static listener; its status is always `active`. Text marks it `· static`. |
| 3 | Non-HCM chains | `NetworkFilterChain.NetworkFilters` lists every network filter (ID, name, type). The HCM is found by name or by `typed_config` type. A chain without an HCM is kept. A network filter with `config_discovery` has `NetworkFilter.Discovery` and stays unresolved. It does not hide an HCM the parser read from another network filter in the same chain. |
| 4 | ECDS and wrapper state | A filter with `config_discovery` has `HTTPFilter.Discovery`. Its config is the `EcdsConfigDump` entry with the filter's name and a type in `type_urls`, joined per category (downstream HTTP, upstream HTTP), with one config source subscribing to that name (`ConfigSource: ecds`). Sections map to categories by Envoy's key order only when every `config_discovery` in the listeners is accounted for and the section count matches. When at least one entry matches, a filter stays unresolved, with an `ecds-ambiguous` warning, if the category is unknown, several sources subscribe to its name, two entries match, or it is a router upstream filter (a cluster may subscribe to the same name, and clusters are not parsed). With no matching entry there is no such warning, and the default is in use (`ecdsDefault`), for an upstream slot too, when `default_config` is set, the listener is active, the capture is complete with no skipped ECDS entry or section, and for a static listener Envoy's workers started. Otherwise `Runs` is unknown and `State` unresolved; a filter the HCM disables and the route doesn't enable is `Runs: no`. An ECDS router's upstream filters come from the config it runs; an unresolved router's are unknown and not listed. `TypedStruct` configs are not unwrapped. Composite and ExtensionWithMatcher set `Wrapper` and `Conditional`. |
| 5 | Matchers | Header matchers read every top-level form (`exact_match`, `prefix_match`, `suffix_match`, `contains_match`, `safe_regex_match`, `present_match`, `range_match`), every `string_match` form including `custom`, `invert_match`, `ignore_case` and `treat_missing_header_as_empty`. No specifier means present. An empty top-level `exact_match` is `anyValue`, not `present`: the two differ under `invert_match`. Query matchers read `string_match` and `present_match`. `HeaderMatch.Kind` and `QueryParamMatch.Kind` name the form; `Regex` stays beside it, true for `regex`. `RouteMatch.CaseSensitive` and `Connect` are read. Predicates the parser doesn't model (gRPC, runtime fraction, dynamic metadata, TLS context, path match policy) are listed in `RouteMatch.Unparsed` and shown in text and JSON. |
| 6 | Default filter chain | The listener's last `FilterChains` entry, `Default: true`, ID `L<n>/Cd`. It has no `TLS`: Envoy ignores its `filter_chain_match`. Text: `default chain <name>`. |
| 7 | Cluster selectors | `cluster_header`, `cluster_specifier_plugin` and `inline_cluster_specifier_plugin` are read into `Route.Action`. `Route.Cluster` stays empty. Text: `backend: from …`. |
| 8 | Route actions | `Route.Action.Kind`: `route`, `redirect`, `directResponse`, `filterAction`, `nonForwarding`. A redirect keeps its status (301 by default) and location parts. A direct response keeps its status and the body's size or source, never the body. |
| 9 | Coverage | `Snapshot.Coverage.Sections` lists every config dump section in dump order: `parsed`, `partial` (entries skipped, or the section failed to decode) or `ignored` (clusters, endpoints, secrets, scoped routes, bootstrap and unknown types; read for `@type` only, empty type when unreadable). Secrets are never decoded. An HCM with `scoped_routes` has `HCMConfig.ScopedRoutes` and use state `unknown`. Text ends with one `ignored dump sections: …` line. |

**Route payload fields.** What a consumer can build its own payload from (`pkg/envoy/model/envoy.go`):

| Type | Fields |
|---|---|
| `RouteMatch` | `Prefix`, `PathSeparatedPrefix`, `Path`, `Regex`, `Connect`, `CaseSensitive *bool` (nil: the dump doesn't set it; Envoy's default is true), `Headers`, `QueryParams`, `Unparsed` |
| `HeaderMatch` | `Name`, `Value`, `Kind`, `Regex`, `IgnoreCase`, `Invert`, `MissingAsEmpty` |
| `QueryParamMatch` | `Name`, `Value`, `Kind`, `Regex`, `IgnoreCase` |
| `MatchKind` | `exact`, `prefix`, `suffix`, `contains`, `regex`, `present`, `absent` (`present_match: false`), `range` (`Value` is `[start,end)`), `custom` (`Value` is the extension's type), `anyValue` |
| `RouteAction` | `Kind`, `ClusterHeader`, `ClusterSpecifierPlugin`, `InlineClusterSpecifierPlugin *Extension` (name and type only), `Redirect`, `DirectResponse`, `FilterActionType` (the action's `@type`) |
| `DirectResponse` | `Status`, `BodySource` (`inline`, `file`, `env`), `BodySize *int` (inline bodies only), `BodyFormat bool` |
| `Route` | also `Rewrite`, `MirrorClusters` (literal cluster names only), `RequestHeadersToAdd`, `ResponseHeadersToAdd`, `ResponseHeadersToRemove`, `WeightedClusters` (ID, name, weight, per-filter config) |

Scope limits:
- A cluster's upstream filters are not read. A cluster subscribing to a router filter's ECDS name from another config source isn't detected.
- A static listener's `Status` stays `active` even before Envoy's workers start. Only the missing-RDS and ECDS-default verdicts check for that.

### 5. Provenance contract (R2, R4, R6, R7)

```go
// Claim is one assertion a module makes about one occurrence.
type Claim struct {
	ID        string          // set by Compose: "<module>/<n>", n the emission index
	Ref       string          // optional module-local name; a nonempty one is unique in the module. A FilterLink refers to it
	Subject   model.ID
	Assertion Assertion
	Certainty model.Certainty // ordered int: unknown < ambiguous < inferred < exact; JSON writes the name
	Evidence  []Evidence
	Module    string          // set by Compose
}

// Evidence is one input a claim rests on.
type Evidence struct {
	Kind    EvidenceKind // metadata, name, table, structure
	Subject model.ID     // the occurrence it was read from
	Field   string       // e.g. "metadata.filter_metadata.merge.TrafficPolicy.gateway.kgateway.dev"
	Object  *model.Ref   // the Kubernetes object the evidence names, if any
}

// Module makes claims about a snapshot. Claims must not modify the snapshot.
type Module interface {
	Name() string
	Claims(snap *model.Snapshot) []Claim
}

// Compose fails only on a programming error: duplicate module names.
func Compose(snap *model.Snapshot, modules ...Module) (*Index, error)
```

`Assertion` is a sealed interface with `Kind() string`; new kinds are additive (rule 7). A claim asserts only what its evidence supports: a fact with different evidence is a separate claim. Caller context never appears in a claim. Phase 9 kinds:

| Kind | Subject | Fields | Cardinality | Certainty cap |
|---|---|---|---|---|
| `listenerSource` | listener, filter chain, route config source, virtual host | port, section (from a name) | alternatives | exact |
| `gatewaySource` | listener, route config source | Gateway ref (from metadata only) | alternatives | exact |
| `hostSource` | virtual host | hostname (`*` when routes have none) | alternatives | exact |
| `routeSource` | route | HTTPRoute ref, rule index (-1 unknown), rule name, match index (-1 unknown), index certainty, incomplete, exhaustive | alternatives | exact |
| `extensionSource` | HTTP filter slot, downstream or router upstream | extension ref, filter kind, helper | alternatives | exact |
| `backendSource` | route or weighted backend | backend ref, port, mirror cluster (empty for the subject's own cluster) | alternatives, per mirror cluster | exact |
| `policyRecord` | listener, route config source, virtual host, route | policy ref, field, level (must match the subject) | cumulative | exact |
| `filterLink` | route | record ref, slot ID | cumulative | inferred |

Go fields (`model.Ref` is `Group`, `Kind`, `Namespace`, `Name`; `Group` is empty for the core group):

| Type | Fields |
|---|---|
| `ListenerSource` | `Port int`, `Section string` |
| `GatewaySource` | `Gateway model.Ref` |
| `HostSource` | `Hostname string` |
| `RouteSource` | `Route model.Ref`, `RuleIndex int`, `RuleName string`, `MatchIndex int`, `IndexCertainty model.Certainty`, `Incomplete bool`, `Exhaustive bool` |
| `ExtensionSource` | `Extension *model.Ref` (nil: no defining object), `FilterKind string`, `Helper bool` |
| `BackendSource` | `Backend model.Ref`, `Port int`, `Mirror string` |
| `PolicyRecord` | `Policy model.Ref`, `Field string`, `Level Level` (`LevelListener`, `LevelRouteConfig`, `LevelVirtualHost`, `LevelRoute`) |
| `FilterLink` | `Record string` (the record claim's `Ref`), `Slot model.ID` |
| `Opaque` | `Name string` (the kind), `Raw json.RawMessage` (the whole assertion object) |

Alternatives compete: for one subject and kind, the highest certainty wins, and several distinct values at that certainty resolve as ambiguous. `routeSource` readings that differ only in index certainty, `Incomplete` or `Exhaustive` are one candidate: exhaustive only if all are, incomplete if any is, with the `IndexCertainty` of the reading whose indices are most certain (unset counts as the claim's certainty and wins a tie; amended after the phase, at kgwctl's report). Cumulative claims are a list; two policy records on one route are two facts.

**JSON.** A claim's assertion is an object with `kind` plus its fields: `{"id": "kgateway/1", "subject": "L0", "assertion": {"kind": "listenerSource", "port": 80}, "certainty": "inferred", "evidence": […], "module": "kgateway"}`. `Claim.UnmarshalJSON` picks the type by `kind`. An unknown kind reads as `Opaque`, which keeps the kind and the raw object and writes it back unchanged, so older readers keep working.

**Compose rules.**
1. Each module gets its own deep copy of `snap`, and `snap` is not changed. A test runs a module that mutates nested maps, slices and pointers on every dump and checks the snapshot, chains and other claims are unchanged (R2). The copy is a safeguard: a module must not modify the snapshot, and `Compose` may drop or thin the copy later.
2. Module names are unique. Within a subject, claims keep module order, then emission order.
3. A pointer assertion is read as its value; a nil one is dropped. A claim is dropped, with a `claim-dropped` warning on the index, when its assertion type is not one of this package's kinds; its subject is not in `snap` or is the wrong kind of occurrence; it has no evidence; an object its assertion names is not the `Object` of any of its evidence; its certainty exceeds its kind's cap; a `PolicyRecord`'s level doesn't match its subject; or its nonempty `Ref` repeats one of its module's. Evidence is checked for presence and for the named objects only: its subjects and kinds are not checked, so krp's JSON projection (§7) can still leave out a kept claim whose evidence subject was pruned. An `Opaque` claim is dropped: no subject kind takes it.
4. A `FilterLink` must name a `PolicyRecord` of the same module that survived rule 3. The record must be on the route, one of its ancestors, or the route config source its use resolves to. The slot must be an HTTP filter of the HCM serving the route, and the route's chain must have per-filter config for that slot at some level or on a weighted backend. `Compose` checks this with `chain.Effective`. When two links from one record on one route pass these checks, both are dropped (`link-dropped`); an invalid link never cancels a valid one.

```go
type Index struct{ /* unexported */ } // read-only, safe for concurrent use

func (ix *Index) About(id model.ID) []Claim
func (ix *Index) Claims() []Claim                       // every claim, in index order
func (ix *Index) Record(link Claim) (Claim, bool)       // the PolicyRecord a FilterLink names
func (ix *Index) Dependencies(block model.ID) Dependencies
func (ix *Index) Inputs(block model.ID) []Input
func (ix *Index) Warnings() []model.Warning             // why claims were dropped

func Resolve[T Assertion](ix *Index, id model.ID) Resolution[T] // BackendSource: the subject's own cluster
func ResolveMirror(ix *Index, route model.ID, cluster string) Resolution[BackendSource]
func Records(ix *Index, snap *model.Snapshot, route model.ID) []Claim // PolicyRecords that apply to a route, most specific first

type Resolution[T Assertion] struct {
	Certainty  model.Certainty // ambiguous when Candidates disagree
	Candidates []T             // distinct assertions at the highest certainty, in claim order
	Claims     []Claim         // every claim of the kind, the outranked ones too
}
```

Index order is subject (`model.CompareID`), then module order, then emission order.

**Ownership.** The index keeps the snapshot passed to `Compose` and builds its dependency data from it on the first `Dependencies` or `Inputs` call. That snapshot, and the nested data in returned claims, are read-only. The index always evaluates the original snapshot: calling it with an ID from a pruned copy answers for the full block, not the pruned view. For a cumulative kind, every assertion is a candidate and `Certainty` is the highest. `Records` returns the records on the route, its virtual host, the source its use resolves to, and its listener; each record's `Level` says where it sits.

#### Dependencies and Inputs

```go
type Dependencies struct {
	Objects []Dependent // every object the block's claims rest on, with the claim kind, sorted
	Gaps    []Gap       // inputs no claim covers, sorted
}

type Dependent struct {
	Object model.Ref
	Kind   string // the claim's assertion kind, e.g. "backendSource"
}

type Gap struct {
	Subject model.ID
	Kind    string   // the assertion kind expected, e.g. "routeSource"
	Slot    model.ID // policyRecord only: the HTTP filter slot, empty when none is installed
	Level   Level    // policyRecord only: the level the record is missing at, empty for a weighted backend
	Cluster string   // backendSource only: the mirror cluster, empty for the route's own backend
}

type Input struct {
	Gap                    // names the input: subject, expected kind, slot, level, cluster
	Status     InputStatus // covered, gap, bootstrap
	Reason     GapReason   // gap only
	Candidates []Candidate // routeSource only: every RouteSource claim on the route, at any certainty
	Complete   bool        // routeSource only: the top readings name every possible source
}

type Candidate struct {
	Ref   model.Ref // the HTTPRoute the claim names
	Claim Claim     // with its evidence and certainty
}
```

`Inputs(block)` enumerates every input the block shows or relies on, sorted by subject, kind, slot, level and cluster, and checks each one against the claim kinds that can cover it. `Dependencies(block)` derives from the same enumeration: its `Gaps` are the inputs with status `gap`. Both are fail-closed: there is no per-case gap rule. `block` is any occurrence ID.

**1. Inputs.**
- Every listener, virtual host, route and weighted backend in or above the block, every mirror cluster of such a route, and its direct response.
- Every network filter other than the HCM in a filter chain in or above the block.
- Every HTTP filter slot, downstream and router upstream, of every HCM serving the block: the HCM in or above it, or for a route config source block, every HCM whose use resolves to it.
- Every per-filter config entry on the source a use in or above the block resolves to (or the source block itself), and on every virtual host, route and weighted backend in or above the block. An entry with no matching installed slot is an input too.
- Every route config use in or above the block whose state is not `found` (`missing`, `ambiguous`, `unknown`, scoped routes). Its routes are not in the dump, so it is always a gap, reason `unresolved`, with an empty `Kind`.
- A route config source block holds every use that resolves to it: the inputs of each use's block (its listener, filter chain, HCM, virtual hosts, routes, backends and mirrors) are its inputs. A static source that an inline source reports is that inline source, with the inline source's uses. A source with no use is a block of its per-filter config entries.

A source block is the union of its uses' blocks. That duplicates the use blocks on purpose: a consumer that shows a shared source once, by `SourcePath`, needs one decision that is never weaker than the decision for any use.

**Bootstrap.** A static listener, everything in it, its inline route configs and the static sources only they report come from the proxy's startup config, not from Kubernetes. They are inputs with status `bootstrap`: never a gap, and they add no objects. Route config that a static listener fetches over RDS is dynamic: its virtual hosts, routes, backends and per-filter config keep their normal inputs and gaps.

**2. Cover.** An input is covered only by claims at inferred or exact. Alternatives kinds cover when they resolve at inferred or exact (`Resolve`), so ambiguous readings are gaps. A filter link covers only on the route being checked. The routes checked are the block's routes beneath the input, or all routes of the HCM when the block holds none. When no route is checked, an input that needs a link stays a gap.

| Input | Covered by |
|---|---|
| listener | `gatewaySource` |
| filter chain, HCM, joined use | nothing expected |
| use not `found` | nothing: always a gap (`unresolved`) |
| virtual host | `hostSource` |
| route | `routeSource`, with a complete candidate list |
| route or weighted backend with a cluster | `backendSource` |
| mirror cluster of a route | `backendSource` on the route naming that mirror (`ResolveMirror`) |
| route whose cluster is picked per request (header, specifier plugin) | nothing: always a gap |
| route with a direct response (its `backendSource` input) | nothing: always a gap. It may come from an object other than the HTTPRoute (kgateway: a DirectResponse resource) |
| network filter other than the HCM | nothing: always a gap |
| HTTP filter slot, downstream or upstream, other than the router | `extensionSource` |
| downstream slot that runs by default, not the router, not marked a helper | also: for each route checked, a kept `filterLink` on that route placing a listener-level `policyRecord` on the slot |
| per-filter config entry for slot S at level X (route, virtual host, route config) | for each route checked, a kept `filterLink` on that route placing a level-X `policyRecord` on S |
| per-filter config entry on a weighted backend, or with no installed downstream slot | nothing: always a gap |

A policy gap names the slot and level. A record for one filter never covers another filter's entry. A default-on slot whose routes have no per-filter config keeps its listener-level gap; a listener-scoped link is the additive fix later (rule 7). Without `merge.*` at a level (U1), a hidden policy could add config there; the gap shows it. Router upstream slots need only `extensionSource`: rule 4 admits no link to them. A mirror's `backendSource` sits on the route and names the mirror, so it competes only with claims for that mirror; its input and gap name it in `Cluster`. No Phase 9 kind has a network filter as subject, and a filter such as RBAC or `tcp_proxy` changes what the route serves, so it is a gap. Each can gain a claim kind later (rule 7).

Cover details:
- An `extensionSource` covers a slot on its certainty alone. It needs no `Extension` object: a `FilterKind` reading is enough.
- "Runs by default" means the HCM doesn't disable the slot (`!HTTPFilter.Disabled`). An unresolved ECDS slot counts.
- "Marked a helper" means any top-certainty `extensionSource` candidate has `Helper`, even when the slot's resolution is ambiguous.
- A placement needs both the link and the record it names at inferred or better.
- Raw `typed_config` and per-filter config values are not attributed to objects. They are checked by entry and placement only.

A claim in scope that Compose rule 3 dropped gives its subject a gap of the dropped claim's kind, reason `dropped`. A dropped mirror claim's gap names its mirror. A dropped claim with a nil assertion, and a link dropped by rule 4, only add an index warning: they make no `dropped` input.

**Input identity.** Inputs merge by the whole `Gap` value (subject, kind, slot, level, cluster). So:
- Several per-filter entries with the same key can be one input, e.g. two entries on one subject that no installed slot takes.
- A merged input is a gap if any part is. A `dropped` reason replaces a cover and any other reason on the same key.
- An entry with no downstream slot names the router upstream slot of the same filter name in `Slot`, when there is one, and stays `notCoverable`.
- A placement with no route to check (an HCM with no routes) is `notCoverable`, and so is every per-filter entry on a weighted backend.

**Gap reasons.**

| Reason | Meaning |
|---|---|
| `ambiguous` | several candidates at the highest certainty |
| `noEvidence` | no claim at inferred or better |
| `dropped` | Compose dropped a claim of this kind on the subject |
| `incomplete` | the route's readings may miss its source |
| `notCoverable` | no claim kind can cover it: a network filter, a per-request cluster, a per-filter entry with no installed downstream slot or on a weighted backend, a placement with no route to check |
| `unresolved` | a route config use joined no source: the routes it serves are unknown |

**Complete.** A route's candidate list is complete only when the module affirms it: every reading at the highest certainty is marked `Exhaustive`, and no listed reading at any certainty, `unknown` included, is marked `Incomplete`. A lower-certainty incomplete reading therefore keeps an exact one from covering the route. A module marks a reading `Exhaustive` when it comes from exact metadata or its parser enumerated every reading of a name. It marks one `Incomplete` when it could not enumerate every reading, such as a name outside the expected format. A list that isn't complete is a gap: `noEvidence` when no reading is at ambiguous or better (an empty list, or only `unknown` readings), otherwise `incomplete`, which wins over `ambiguous`. Candidates are listed whether or not their objects exist in a cluster: xdsview never reads cluster state.

**3. Objects.** `Objects` holds the evidence objects of every claim on an input's subject, on the route config sources in scope and on the serving HCMs' slots, inherited policy records included. That includes claims that are outranked or don't cover anything. Every claim that covers an input adds its evidence objects too, wherever its subject lives. Entries are unique by `(Object, Kind)`.

**4. Cost.** The index builds its per-HCM structure, link placements and bootstrap set once, on the first call. A route block's inputs are its lineage and its HCM's slots, so checking every route block is linear in routes. On a 5,000-route dump, `Compose` takes about 40 ms and 85 MB, and `Dependencies` on every route block about 27 ms more (phase-9.3-complete.md, Benchmark).

**Consumer use.** kgwctl's route lookup is `Resolve[RouteSource](ix, routeID)`. Snellius chooses its own visibility rule from `Inputs`: each input's status, reason and route candidates. The strictest rule withholds a block when the viewer can't read every object in `Dependencies(block).Objects`, or when `Gaps` is not empty (R6). kgateway writes no Gateway metadata, so every kgateway listener has a `gatewaySource` gap (`noEvidence`). A consumer that knows the Gateway it captured may close that gap as its own context. Listeners from a ListenerSet are not attributed in Phase 9. Static listeners and their inline route configs are bootstrap, but route config a static listener fetches over RDS is claimed, and `Select` can pick its routes.

**Tests.** For every dump and fixture and every listener, virtual host, route and route config source block:
1. An oracle marshals the snapshot and chains to JSON and walks them, independently of the index. It requires an input for every item the block shows: each cluster (primary, weighted, mirror, per-request selector), network filter, HTTP slot, per-filter config entry, listener, virtual host and route.
2. Removing the claims that cover an input from the kgateway module's output adds that input's own gap. Redundant covers of one input are removed together.
3. `Dependencies(block).Gaps` equals the gap-status inputs of `Inputs(block)`.

**Stability.** The Go shapes of `Dependencies` and `Inputs` follow P-010 rule 7. Their results do not yet: the input list, the cover table and the gap reasons may change in a minor release until Snellius Phase 12 is designed. A new claim kind, parsed section or model field can then show or hide a block with no Go break.

**Raw metadata.** Listener, virtual host, route and source `Metadata` stays in the snapshot and in krp's JSON as Envoy data. It is not an attribution input: no input or gap derives from it, so a `merge.*` entry the module skips leaves its name in `Metadata` without a claim. A consumer that applies per-object visibility must not render raw `Metadata` to a restricted viewer.

### 6. kgateway module (R5, R8, R9, R13)

`kgateway.New() provenance.Module`, named `kgateway` (`ModuleName`). It takes no caller context: a consumer that knows which Gateway it captured shows that itself, labelled as its own context.

- Each claim lists its evidence: a name (`name`), `route_source` or `merge.*` metadata (`metadata`), the field table (`table`), a filter's type (`structure`).
- `gatewaySource` comes from metadata only. kgateway writes none, so the module emits none.
- `listenerSource` comes from listener, filter chain, virtual host and route config names. A dynamic listener gets one only from a `listener~<port>` name. Standalone static route config sources (`RS<n>`) get no claims.
- `merge.*` is decoded at listener, route config, virtual host and route level. A policy ID of any group and kind is read when it is valid and agrees with its key; only kgateway's `TrafficPolicy` gets filter links. Each record is a `policyRecord` on its own level. Nothing says which level governs (P-008). Scenario `10-policy-levels` (kgateway v2.5.2) has records at every level, in the same key and value shape as the hand-built fixtures.
- Route config that a static listener fetches over RDS is claimed like any other. Static listeners and their inline configs get no claims (bootstrap).
- Field links follow rule 4: one matching slot or none. Any Composite delegate is a candidate (#5). With `dynamic_module_config.module` set, a link needs a local module file with the expected name; a remote module gets no link (#4).
- A route name is parsed after its own `<vhost>-route-<N>-` prefix, so a marker inside the virtual host name can't change the readings. Every split of the rest is enumerated, and those readings are `Exhaustive`. A name without that prefix is read after every possible prefix, and its readings are `Incomplete`. A rule name with dots reads its rule index and name (#2).
- Readings from `route_source` metadata are exact and `Exhaustive`, for any route kind the metadata names. Missing indices are filled only from the first name reading with the same route identity, and only when it agrees with the metadata. Then `IndexCertainty` is inferred.
- A Service cluster name needs DNS-label namespace and name and an unsigned decimal port.
- Policy records come out in a deterministic order.

**Version-keyed tables (R14).** `PolicyTableVersion` (`v2.5.1`) names the kgateway version the policy field table was checked against; `PolicyFields()` lists its fields. `headerModifiers` links to the `header_mutation` filter. `CheckedMinors()` (`v2.4`, `v2.5`) lists the minors whose name formats and metadata shapes the module was checked against. Tests:
1. `TestPolicyTable_CoversCorpus`: every field in the corpus's `merge.*` records is in the table. There are no exclusions.
2. `TestVersionTables_MatchCorpus`: `CheckedMinors()` equals the recorded kgateway minors, and `PolicyTableVersion` has a recording.

**Selection (R8).**

```go
func Select(ix *provenance.Index, snap *model.Snapshot, q Query) Selection

type Query struct {
	Route     model.Ref   // an HTTPRoute (gateway.networking.k8s.io)
	RuleIndex *int        // nil: any rule. With RuleName, both must match
	RuleName  string      // empty: any rule
	Known     []model.Ref // HTTPRoutes that exist; narrows readings (rules below)
}

type Selection struct { // every list in snapshot order
	Routes     []model.ID  // one reading, matching the query, not narrowed by Known
	Ambiguous  []model.ID  // several readings, or one left only by Known; at least one matches
	Unresolved []model.ID  // no RouteSource claim, and the name contains "httproute-<name>-<ns>-[<rule index>-]"; RuleName is not checked
	Narrowed   []Narrowing // the routes in the lists above that Known narrowed
}

// Narrow applies Known to every route's readings, without a query, by the
// rules of Select. It returns the routes Known narrowed, in ID order.
func Narrow(ix *provenance.Index, known []model.Ref) []Narrowing

// In provenance; kgateway.Narrowing is an alias.
type Narrowing struct {
	Route   model.ID
	Kept    []Claim // RouteSource claims Known kept
	Dropped []Claim // RouteSource claims Known removed
}
```

Selection rules:
1. `Query.Route` is an HTTPRoute. Other route kinds are not selected; they are additive later.
2. A reading matches when its HTTPRoute equals `Query.Route`, and its rule index and rule name match where set.
3. `Routes`: the route resolves to one reading at inferred or exact, it matches, and Known removed no reading. Readings are counted as `Resolve` merges them: one reading is one HTTPRoute, rule index, rule name and match index, so claims that differ only in `IndexCertainty`, `Incomplete` or `Exhaustive` are one reading.
4. `Ambiguous`: several readings and at least one matches, counting those Known removed; or one reading left only because Known removed the others; or one matching reading whose claims are at `ambiguous` certainty (the module itself says it is unsure). Ambiguous routes are never selected.
5. Known removes a reading only when its HTTPRoute is absent from Known. It never removes an exact reading, and never the last one: when every reading is outside Known, Known is ignored for that route and no narrowing is recorded.
6. Known never raises certainty. There is no `narrowed` certainty: a narrowed route stays ambiguous, and its `Narrowing` names what was kept.

A static listener's inline routes have no `routeSource` claims, so `Select` never picks them. Routes a static listener fetches over RDS are claimed like any other and can be selected. Consumers prune with `model.Prune(snap, in(sel.Routes))`; the index keeps every candidate.

**Names (R9).** `kgateway.Names() provenance.NameMapper`. It takes no options.

```go
type NameMapper interface {
	Name(kind NameKind, name string) NameAnswer // kind: cluster, routeConfig, listener
}

type NameAnswer struct {
	Certainty  model.Certainty
	Candidates []model.Ref // several when ambiguous; each carries its own kind
}
```

A cluster `kube_<ns>_<name>_<port>` is a Service, inferred. Route config and listener names, RDS names included, carry a port or listener name but no object, so they answer unknown; a consumer maps them with its own Gateway context. An ambiguous answer lists every candidate the name can read as, never a subset. A consumer that supports several kgateway minors checks its minor against `CheckedMinors()`. There is no route-name kind and no kind-guessing helper: every consumer call site knows its kind.

### 7. Rendering and krp (R10)

`render.Text(w io.Writer, snap *model.Snapshot, chains map[model.ID]model.EffectiveChain, opts Options) error` writes the tree. A filter row appears once per route, so hooks key on a row, not a slot:

```go
// Row names one printed row. Subject is the occurrence the row shows:
// listener, filter chain, virtual host, route or slot. Slot is empty above
// filter level; Route is empty above route level.
type Row struct{ Subject, Route, Slot model.ID }

type Options struct {
	Header        string
	ResponseOrder bool // also print each route's response order
	AllFilters    bool // list filters that don't run inline; default: one "not run:" line

	Style   func(c Class, r Row, s string) string
	Notes   func(r Row) []string // lines under a route or filter row; a route's come before its filters
	Labels  func(r Row) []string // text on the row itself, each after " · "
	Trailer func(r Row) []Line   // labelled lines after a route's filters, before its actions and backend
}

// Line is one labelled line, printed as "label: text" with the text styled ClassOrigin.
type Line struct{ Label, Text string }

type Class int // ClassListener, ClassVirtualHost, ClassRoute, ClassFilter, ClassFilterOff,
               // ClassOrigin, ClassBackend, ClassWarning, ClassFilterChain
```

`Labels` and `Notes` run on numbered filter rows, including an HCM default chain's rows, whose `Route` is empty. Summary lines (`not run:`, ignored sections) and upstream filter lines don't call them. `provenance.Labels` prints no filter attribution on a row with an empty `Route`.

`render.Text` prints a subject's warnings as `warning: …`, upstream-slot warnings included, and one coverage line for ignored sections. A warning appears under its subject's row. An unjoined use's warning is under its `route config …` line, whose `Row.Subject` is the use. When text skips the level of a listener's one plain filter chain, the chain's warnings appear under the listener row. In a chain without an HCM, network filter warnings appear under the chain's line. Dump-wide warnings stay with the caller.

`provenance` supplies the hooks for its claims, so krp and kgwctl print the same text:
- `provenance.Labels(ix, narrowings)` is the `Labels` hook. A listener row shows its port or section; a virtual host its listener; a route its reading (`HTTPRoute ns/name · rule 0 · match 0`, a narrowed one marked `(chosen from known HTTPRoutes)`, one with indices from the name marked `(partly from name)`). A filter row shows the extension that defines it and the policy records linked to it on that route, each policy field once with its levels, most specific first. A route-only link prints no level. Inferred labels say `inferred`; exact ones print bare.
- `provenance.Notes(ix)` is the `Trailer` hook: `policies:` for the route's own records and `policies recorded above the route:` for those at its virtual host, route config and listener (renamed from `inherited policies:` after the phase, at kgwctl's request).

A test puts one slot under two routes with different per-filter config and links, and checks each row gets its own text.

**krp flow.** Parse → `provenance.Compose(snap, kgateway.New())` → `chain.Effective` for krp's own chain map → `kgateway.Select` under `--route` (Known is the `--route` HTTPRoute), or `kgateway.Narrow` in live mode with the cluster's HTTPRoutes → `model.Prune`, under `--route` only → render with `Labels` and `Notes` as `Trailer`. Index warnings go to stderr. The TUI reads the same results.

**krp's selection choices (D-022).**
1. krp also selects a route that `Select` leaves ambiguous when its `Narrowing` kept exactly one reading and that reading matches the query. The text marks it chosen from known HTTPRoutes. `kgateway.Select` keeps the strict rule.
2. With `--gateway`, a non-static listener row ends with `Gateway ns/gw (from --gateway)`: krp's own context, never a claim.

**krp `-o json` `v1alpha2`.**

```json
{
  "schemaVersion": "v1alpha2",
  "snapshot": { "capture": "unverified", "listeners": [], "routeConfigs": [], "coverage": {} },
  "chains": { "L0/C0/N0/RC/V0/R0": { "filters": [] } },
  "claims": [ { "id": "kgateway/0", "subject": "…", "assertion": { "kind": "routeSource" }, "certainty": "exact", "evidence": [ { "kind": "metadata", "subject": "…", "field": "…" } ], "module": "kgateway" } ],
  "selection": { "query": {}, "routes": [], "ambiguous": [], "unresolved": [], "narrowed": [] },
  "warnings": [ { "subject": "…", "code": "…", "message": "…" } ]
}
```

| Field | Content |
|---|---|
| `snapshot` | The (pruned) snapshot. `listeners` is always an array. `snapshot.warnings` and an empty `routeConfigs` are omitted: warnings are in the envelope. Nested nil arrays, such as an HCM's `httpFilters` or a route config's `virtualHosts`, can be `null`. |
| `chains` | Each route's chain by route ID, and an HCM's default chain by its use ID when no route config joined. A chain's `filters` can be `null`. |
| `claims` | Every kept claim whose subject, evidence subjects and link slot are in `snapshot`. A link stays only with the record it names. A composed claim always has at least one evidence entry. |
| `selection` | With `--route`: `query`, `routes` (what krp prints, D-022 routes included), `ambiguous` and `unresolved` (what it leaves out), `narrowed`. `query` has `route` (a `model.Ref`), and `ruleIndex` (integer) and `ruleName` when set. In live mode without `--route`: no `query`, and `narrowed` only, the other lists empty. Absent otherwise. |
| `warnings` | Parse warnings and the index's, typed. An index warning whose subject the dump lacks keeps its message without a subject. |

Every field except `selection` is projected onto the retained subjects: no ID outside `selection` is absent from `snapshot`. `selection` describes the whole dump. Its `ambiguous`, `unresolved` and `narrowed` entries carry `id`, `name` and `virtualHost`, and their IDs may be absent from `snapshot`. A `narrowed` entry embeds its `kept` and `dropped` claims, not claim IDs, and a D-022 route has `"selected": true`. A `filterLink`'s `assertion.record` is the record claim's `ref`: join it on the pair `(module, ref)`, not on the claim `id`. `schemaVersion` changes only when a field is removed or renamed; new assertion kinds may appear without it. The README's JSON section is the schema doc and lists each assertion kind.

## Compatibility

| Consumer | Before | Effect |
|---|---|---|
| kgwctl `proxy routes` (its Phase 4 checkout pins `v0.4.3`) | `parser`, `origin.Kgateway`, `origin.Annotate`, `chain.Annotate`, `filter.Filter`/`Ambiguous`, `render.Text` with `RouteRef`/`FilterRef`, embeds `model.EnvoySnapshot` in its payload | Migrates in one step (E15): builds `ProxyRoutes` from snapshot, chains and index; route IDs and `SourcePath` replace its `(routeConfigName, routeName)` dedup, which collapses shared route configs; bumps its envelope to `v1alpha2`. Text, JSON and YAML goldens change. |
| Snellius Phase 12 | no import | Targets `v0.7.0`. Uses `Inputs` and `Dependencies` to decide what to withhold, and `NameMapper` for `EnvoyNamer`. |
| krp JSON users | `v1alpha1` | `v1alpha2`; no compatibility shim. |
| downstream product module | `origin.Chain` plugin | Implements `provenance.Module`. Its old plugin breaks. |

Every exported package except `source` changes. No new dependencies.

## Data-plane cost

n/a: read-only tool.

## Risks

- **Scoped `merge.*` shape drifts.** One recorded kgateway version (v2.5.2, scenario 10) shows records above route level. A later minor may change the shape; the version tests (§6) only check that a recording exists.
- **Hand-built fixtures.** The control-plane-neutral core paths (ECDS, default chain, selectors, actions, matchers, tcp_proxy, static bootstrap, coverage) are tested on hand-built dumps. They prove the parser matches the authors' reading of Envoy's JSON, not Envoy's output.
- **ID churn surprises consumers.** IDs shift when a sibling is added. The doc comment, the JSON schema notes and the consumer notices say IDs are per dump.
- **Deep copy per module.** A large dump is copied once per module. `Compose` with one module costs 85 MB at 5,000 routes; the copy's share is not measured. A module must not rely on the copy (§5 rule 1).
- **Gaps on real output.** Without `route_source` metadata, kgateway route names read several ways, so most kgateway routes have a `routeSource` gap. A consumer's strictest rule then shows little; `Inputs` lets it choose a looser one.
