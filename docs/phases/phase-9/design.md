# Phase 9 — Control-Plane-Agnostic Contract: design

**Status:** approved by the owner (2026-11-25); amended after the Codex design review (reviews/design-codex.md). kgwctl and Snellius review pending
**Edge:** E15 | **Program:** P-010 (contract), P-011 rule 6 (snapshot-local IDs), P-012 rule 4 (version-keyed tables); the last two in program.md "Architecture guidelines" (`dbf7140`) | **Decisions:** D-019, D-020
**Release:** one tag at phase close, `v0.7.0`: the break plus core correctness (D-020)
**Inputs:** [approach consult](reviews/approach-codex.md), Phase 8 deferrals ([phase-8-complete.md](../phase-8/phase-8-complete.md)), E14 missing RDS ([xds-convergence.md §4.1](../../investigations/xds-convergence.md))

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
| core | `pkg/envoy/model` | snapshot types with IDs, `ID`, `Certainty`, `Ref`, `Prune` | stdlib |
| core | `pkg/envoy/parser` | config dump → `*model.Snapshot` | model |
| core | `pkg/envoy/chain` | `Effective`: effective chain per route | model |
| core | `pkg/envoy/render` | text view of a snapshot and its chains; generic note hooks | model |
| core | `pkg/envoy/source`, `source/kube` | as today, plus `Capture()` on `Source` | as today |
| contract | `pkg/envoy/provenance` | `Claim`, assertion kinds, `Module`, `Compose`, `Index`, `NameMapper`, `Notes` | model, chain, render |
| module | `pkg/adapter/kgateway` | today's `origin/kgateway.go` and `filter`: claims, `merge.*` and `route_source` decoding, route selection, names | model, provenance |

`origin` and `filter` are removed. A dependency test fails the build if `model`, `parser`, `chain` or `render` import `provenance` or `adapter/...` (extends the D-004 test).

### 2. Snapshot and occurrence IDs (R3, R11)

`parser.Parse(r, opts) (*model.Snapshot, error)`. Warnings move into the snapshot, typed and keyed by ID.

The same bytes can come from a complete or a filtered dump (design review #2). `model.Capture` says which: `CaptureUnverified` (the default, and every file input unless the caller states otherwise) or `CaptureComplete`. `source.Source` gains `Capture() model.Capture`: `Admin` and `kube` return `CaptureComplete` when they fetched `/config_dump` with no filter or mask, `File` returns unverified. The caller passes it in `parser.Options.Capture`, and `Snapshot.Capture` carries it. Missing-RDS (§4.1) needs `CaptureComplete`.

```go
// ID names one occurrence in one Snapshot. It is opaque and valid only in
// the snapshot that produced it (program P-011 rule 6).
type ID string

// Within reports whether id is block or lies inside it.
func (id ID) Within(block ID) bool

type Snapshot struct {
	Listeners    []Listener
	RouteConfigs []RouteConfigSource // every RDS and static entry, in dump order
	Coverage     Coverage            // §4.9
	Warnings     []Warning
}

type Warning struct {
	Subject ID     // empty for dump-wide warnings
	Code    string // e.g. "rds-missing", "section-skipped"
	Message string
}
```

IDs are assigned once, in `parser.Parse`, as structural paths: `L0`, `L0/C1`, `L0/C1/N0` (a network filter), `L0/C1/N0/F3` (an HTTP filter slot), `L0/C1/N0/U0` (upstream filter), `L0/C1/N0/RC` (the HCM's route config use), `.../RC/V2`, `.../RC/V2/R5`, `.../R5/W1`. The HCM is its network filter's occurrence: it has no ID of its own. Consumers compare IDs and call `Within`; they never parse them.

**Route config sources and uses** (design review #7). A source is one route configuration as the dump holds it: `RD<n>` for a `dynamic_route_configs` entry, `RS<n>` for a `static_route_configs` entry, `<HCM>/RCI` for an HCM's inline `route_config`. A use (`<HCM>/RC`) has `Source ID`, empty when unresolved, and `RouteConfigState`: `found`, `missing`, `ambiguous` or `unknown`. RDS names resolve against dynamic sources only. A static entry with the same name and content as an inline config is the same configuration as Envoy reports it: the inline source records the static ID as `Reported`. Any other static entry stays a source with no use. Claims about route-configuration-level data (`merge.*`, per-filter config) are on the source; a use inherits them (§5, `Dependencies`).

Every type that has an ID carries it as a field. `model.Prune(snap, keep func(ID) bool)` returns a copy that keeps the selected routes and their ancestors. It also keeps every listener that today's `filter` keeps: any listener not active, and an active one that is warming a replacement, draining an older version or has a rejected update (design review #4; kgwctl tests the mid-update case). IDs survive pruning, so any result computed on the full snapshot still joins.

A route config shared by two HCMs has one source (`RD3`) and two uses (`L0/C1/N0/RC`, `L1/C0/N0/RC`), each with its own virtual host and route IDs (Codex #5). Two sources with the same name are both kept. An HCM that references that name gets `RouteConfigState` `ambiguous` and no join, with a warning; today the later entry silently wins ([parser.go:100](../../../pkg/envoy/parser/parser.go)).

Provenance fields leave the model: `Listener.Origin`, `HTTPFilter.Origin`, `VirtualHost.Origin`, `Route.Origin`, `Route.Policies` and `Route.Chain` are removed.

### 3. Execution (R1, R11)

```go
// Effective resolves the chain of every route in snap. It reads snap only.
func Effective(snap *model.Snapshot) map[model.ID]model.EffectiveChain
```

Each `ChainEntry` gains `Slot model.ID` and loses `Contributions`. Run state becomes three-valued:

```go
type Tri string // "yes", "no", "unknown"

type ChainEntry struct {
	Slot          model.ID
	Name, Type    string
	Runs          Tri         // unknown: ECDS without config, rejected per-filter config
	State         FilterState // adds FilterUnresolved (ECDS without config)
	Level         ConfigLevel
	PerFilterType string
	Request       Tri
	Response      Tri
	Wrapper       string // "composite" or "extensionWithMatcher"; empty otherwise
	Conditional   bool   // matcher-dependent: a match can pick a delegate or skip the filter
	ConfigSource  string // "inline", "ecds", "ecdsDefault"; empty when unresolved
}
```

`RequestOrder` and `ResponseOrder` keep entries whose run state or side is `unknown`, as today for sides. Text output shows them in position with an uncertainty label, never as running (design review #9). The sides table stays in `chain` (modules never supply sides, rule 3) and names its Envoy version (R14). Its check: a test fails when a filter type in any fixture is missing from the table without a recorded exclusion.

### 4. Core correctness (R12)

Each item changes OSS output. Goldens change only through reviewed `-update` runs.

| # | Item | Behaviour |
|---|---|---|
| 1 | Missing RDS (E14) | An active listener's HCM with an RDS name and no matching dynamic source gets `RouteConfigState` `missing` and a `rds-missing` warning, only on a `CaptureComplete` snapshot. It is `unknown` when the capture is unverified, `RoutesConfigDump` is absent, or any route config entry was skipped. Warming and draining listeners stay `unknown`. Tests cover complete, unverified (filtered) and draining cases. |
| 2 | Static listeners and route configs | `static_listeners` and `static_route_configs` are parsed (§2 sources and uses). `Listener.Static` marks them. |
| 3 | Non-HCM chains | Every filter chain lists its network filters (ID, name, type). A chain without an HCM is kept, not dropped. |
| 4 | ECDS and wrapper state | A filter with `config_discovery` takes its config from the `EcdsConfigDump` entry whose name is the filter's name and whose type is in `type_urls` (`ConfigSource: ecds`). With no entry, on an active listener with `default_config` set, the default is in use: Envoy applies it when discovery fails, and `apply_default_config_without_warming` only decides whether it waits (`ecdsDefault`). Otherwise the filter stays in place: `Runs` unknown, `State` unresolved. Composite and ExtensionWithMatcher set `Wrapper`; both set `Conditional`. |
| 5 | Matchers | Header matchers read the top-level `exact_match`, `prefix_match`, `suffix_match`, `contains_match`, `safe_regex_match`, `present_match` and `range_match`, every `string_match` form, `invert_match` and `ignore_case`. Query matchers read `string_match` and `present_match`. `RouteMatch.CaseSensitive` and `connect_matcher` are read. Predicates the parser doesn't model (gRPC, runtime fraction, dynamic metadata, TLS context, path match policy) are listed in `RouteMatch.Unparsed` and shown in text and JSON, never dropped. |
| 6 | Default filter chain | `Listener.DefaultFilterChain` is parsed with its own ID (`L0/Cd`). |
| 7 | Cluster selectors | `cluster_header`, `cluster_specifier_plugin` and `inline_cluster_specifier_plugin` are read into `Route.Action`. The route has no cluster name, and says why. |
| 8 | Route actions | `Route.Action.Kind`: `route`, `redirect`, `directResponse`, `filterAction`, `nonForwarding`. Redirect and direct response keep their fields (status, location parts; body size, never the body). |
| 9 | Coverage policy | `Snapshot.Coverage` lists every config dump section: `parsed`, `partial` (entries skipped), or `ignored` (clusters, endpoints, secrets, scoped routes, and unknown types). Secrets are never decoded. An HCM with `scoped_routes` gets `RouteConfigState` `unknown`. Text output prints one line for ignored sections. |

### 5. Provenance contract (R2, R4, R6, R7)

```go
// Claim is one assertion a module makes about one occurrence.
type Claim struct {
	ID        string          // set by Compose: "<module>/<n>"
	Ref       string          // module-local name, unique in the module; FilterLink refers to it
	Subject   model.ID
	Assertion Assertion
	Certainty model.Certainty
	Evidence  []Evidence
	Module    string          // set by Compose
}

// Evidence is one input a claim rests on.
type Evidence struct {
	Kind    EvidenceKind // metadata, name, table, structure
	Subject model.ID     // the occurrence it was read from
	Field   string       // e.g. "metadata.filter_metadata.merge"
	Object  *model.Ref   // the Kubernetes object the evidence names, if any
}

type Module interface {
	Name() string
	Claims(snap *model.Snapshot) []Claim
}

// Compose fails only on a programming error: duplicate module names.
func Compose(snap *model.Snapshot, modules ...Module) (*Index, error)
```

`Assertion` is a sealed interface; new kinds are additive (rule 7). A claim asserts only what its evidence supports: a fact with different evidence is a separate claim (design review #5). Caller context never appears in a claim. Phase 9 kinds:

| Kind | Subject | Fields | Cardinality | Certainty cap |
|---|---|---|---|---|
| `ListenerSource` | listener, filter chain, route config source | port, section (from the name) | alternatives | as read |
| `GatewaySource` | listener, route config source | Gateway ref (from metadata only) | alternatives | as read |
| `HostSource` | virtual host | hostname | alternatives | as read |
| `RouteSource` | route | HTTPRoute ref, rule index, rule name, match index, index certainty | alternatives | as read |
| `ExtensionSource` | HTTP filter slot | extension ref, filter kind, helper | alternatives | as read |
| `BackendSource` | route or weighted backend | backend ref, port | alternatives | as read |
| `PolicyRecord` | listener, route config source, virtual host, route | policy ref, field, level | cumulative | exact (validated `merge.*`) |
| `FilterLink` | route | record ref, slot ID | cumulative | inferred |

Alternatives compete: for one subject and kind, the highest certainty wins, and several distinct values at that certainty resolve as ambiguous. Cumulative claims are a list; two policy records on one route are two facts.

Compose rules:
1. Each module gets its own deep copy of `snap`. A test runs two modules, one of which mutates nested maps, slices and pointers, and checks the original is unchanged (R2).
2. Module names are unique. Claims keep module order, then emission order.
3. A claim is dropped, with a warning on the index, when its subject is not in `snap`, it has no evidence, an object its assertion names is not the `Object` of any of its evidence, or it exceeds its kind's cap. A fact with different evidence is a separate claim, so fields with separate evidence never compete as one value.
4. A `FilterLink` is resolved by the composer (design review #6). Its record ref must name a `PolicyRecord` of the same module that survived rule 3. The record's subject must be the route, one of its ancestors, or the route config source its use resolves to. Its slot must be in the HCM that serves the route, and the route's chain must have per-filter config for that slot at some level or on a weighted backend (today's rule, `chain/annotate.go:66`). The composer computes this with `chain.Effective`. Two links from one record on one route drop both.

```go
type Index struct{ /* unexported */ }

func (ix *Index) About(id model.ID) []Claim
func Resolve[T Assertion](ix *Index, id model.ID) Resolution[T]
func (ix *Index) Dependencies(block model.ID) Dependencies
func (ix *Index) Warnings() []model.Warning

type Resolution[T Assertion] struct {
	Certainty  model.Certainty // ambiguous when Candidates disagree
	Candidates []T
	Claims     []Claim
}

type Dependencies struct {
	Objects []model.Ref // every object the block's claims rest on, sorted
	Gaps    []Gap       // expected attribution that is missing or ambiguous
}

type Gap struct {
	Subject model.ID
	Kind    string // the assertion kind expected, e.g. "routeSource"
}
```

`Dependencies(block)` covers everything the block shows and relies on (design review #1):
1. Claims on the block and its descendants.
2. Claims on its ancestors up to the listener, and on the route config source of every use in or above the block, including a source shared with HCMs outside it. Inherited policy records count.
3. Claims on the HTTP filter slots of the HCM that serves it.

A gap is recorded per expected kind, not per subject: a route without `RouteSource`, a slot with per-filter config and no `ExtensionSource`, a route or backend with a cluster and no `BackendSource`, each below inferred or ambiguous. A policy claim on a route doesn't hide a missing `RouteSource`.

kgwctl's route lookup is `Resolve[RouteSource](ix, routeID)`. Snellius withholds a block when the viewer can't read every object in `Dependencies(block).Objects`, or when `Gaps` is not empty (R6).

### 6. kgateway module (R5, R8, R9, R13)

`kgateway.New() provenance.Module`. It takes no caller context. `KgatewayOptions.Gateway` is gone: a consumer that knows which Gateway it captured shows that itself, labelled as its own context. `Known` moves to selection.

- Today's mapper logic becomes claim producers. Each claim lists its evidence: route name (`name`), `route_source` metadata (`metadata`), the field table (`table`).
- `merge.*` is decoded at listener, route config and virtual host level, beside route level. Each record is a `PolicyRecord` on its own level. Nothing says which level governs (P-008).
- Field links follow rule 4: one matching slot or none. A Composite's delegates are candidates too (#5). The rustformation check reads the field Envoy loads the module from; when `name` and `module` disagree, there is no link (#4).
- A route name whose rule name contains dots reads its rule index and name (#2).
- The policy-field table names the kgateway version it was checked against. Its check: every field in the corpus's `merge.*` records is in the table or a recorded exclusion (R14, #30).

Selection (R8):

```go
func Select(ix *provenance.Index, snap *model.Snapshot, q Query) Selection

type Query struct {
	Route     model.Ref
	RuleIndex int // or AllRules
	RuleName  string
	Known     []model.Ref // HTTPRoutes that exist; used to narrow readings
}

type Selection struct {
	Routes     []model.ID  // selected
	Ambiguous  []model.ID  // could match, not selected
	Unresolved []model.ID  // carry a route-name marker that could name the queried HTTPRoute and rule, but no valid reading
	Narrowed   []Narrowing // a reading dropped because its HTTPRoute isn't in Known
}

// Narrow applies Known to every route's readings, without a query.
func Narrow(ix *provenance.Index, known []model.Ref) []Narrowing
```

`Known` only removes readings. It never establishes a match on its own: a route whose readings all fall outside `Known` stays unresolved, not narrowed to nothing (kgwctl `view.go:114`, design review #8). krp's live mode without `--route` calls `Narrow` and shows the narrowed reading with its note, as today. A reading that `Known` alone leaves single keeps its ambiguity label (`narrowed`); its certainty is not raised.

Consumers prune with `model.Prune(snap, in(sel.Routes))`. The index keeps every candidate.

Names (R9): `kgateway.Names(opts) provenance.NameMapper`.

```go
type NameMapper interface {
	Name(kind NameKind, name string) NameAnswer // kind: cluster, routeConfig, listener
}

type NameAnswer struct {
	Certainty  model.Certainty
	Kind       string      // resource kind of the candidates, e.g. "Service"
	Candidates []model.Ref // several when ambiguous
}

// Guess tries every kind, for callers with a bare name and no kind
// (Snellius EnvoyNamer). Answers for different kinds make it ambiguous.
func Guess(m NameMapper, name string) NameAnswer
```

### 7. Rendering and krp (R10)

`render.Text(w, snap, chains, opts)`. A filter row appears once per route, so hooks key on a row, not a slot (design review #3):

```go
// Row names one printed row. Slot is empty for a route row; Route is empty
// above route level.
type Row struct{ Subject, Route, Slot model.ID }

Style func(c Class, r Row, s string) string
Notes func(r Row) []string // lines under the row; kgwctl's narrowing note is a route-row note
Labels func(r Row) []string // text on the row itself, e.g. today's "(policy TrafficPolicy ns/name, inferred)" (9.2 complete, deviation 2)
```

`provenance.Notes(ix)` returns a `Notes` function with today's origin, policy and contribution lines, so krp and kgwctl print the same text. A test puts one slot under two routes with different per-filter config and links, and checks each row gets its own.

krp flow: parse → `chain.Effective` → `provenance.Compose(snap, kgateway.New())` → `kgateway.Select` when `--route` is set, else `kgateway.Narrow` in live mode → `model.Prune` → render. The TUI reads the same results.

Pruned output projects every result onto the retained subjects: chains, claims and warnings for subjects that remain. Prune keeps the route config source of every retained use, so evidence subjects stay present; a claim whose evidence names a dropped subject is dropped from the output. `selection` is the exception: it describes the full snapshot, so its ambiguous and unresolved entries carry the route name and vhost name beside the ID, and the schema says those IDs may be absent from `snapshot` (design review r2).

krp `-o json` `v1alpha2`:

```json
{
  "schemaVersion": "v1alpha2",
  "snapshot": { "listeners": [], "routeConfigs": [], "coverage": {} },
  "chains": { "L0/C0/N0/RC/V0/R0": { "filters": [] } },
  "claims": [ { "id": "kgateway/0", "subject": "…", "kind": "routeSource", "certainty": "exact", "evidence": [], "module": "kgateway" } ],
  "selection": { "routes": [], "ambiguous": [], "narrowed": [] },
  "warnings": [ { "subject": "…", "code": "…", "message": "…" } ]
}
```

## Compatibility

| Consumer | Today | Effect |
|---|---|---|
| kgwctl `proxy routes` (its Phase 4 checkout pins `v0.4.3`) | `parser`, `origin.Kgateway`, `origin.Annotate`, `chain.Annotate`, `filter.Filter`/`Ambiguous`, `render.Text` with `RouteRef`/`FilterRef`, embeds `model.EnvoySnapshot` in its payload | Migrates in one step (E15): builds `ProxyRoutes` from snapshot, chains and index; route IDs replace its `(routeConfigName, routeName)` dedup, which collapses shared route configs; bumps its envelope to `v1alpha2`. Text, JSON and YAML goldens change. |
| Snellius Phase 12 | no import today | Targets `v0.7.0`. Uses `Dependencies` to withhold blocks and `NameMapper` for `EnvoyNamer`. |
| krp JSON users | `v1alpha1` | `v1alpha2`; no compatibility shim. |
| downstream product module | `origin.Chain` plugin | Implements `provenance.Module`. Its old plugin breaks. |

Every exported package except `source` changes. No new dependencies.

## Data-plane cost

n/a: read-only tool.

## Risks

- **Listener, route config and vhost `merge.*` may not appear in OSS dumps.** If no capture shows them, R5 is tested on hand-built fixtures only, and that is stated. A kind capture is tried first.
- **Size.** The break touches every core package. The plan splits it into sub-phases that each keep tests green; the tag waits for the last one.
- **ID churn surprises consumers.** IDs shift when a sibling is added. The doc comment, the JSON schema notes and the kgwctl and Snellius review request all say IDs are per dump.
- **Deep copy per module.** A large dump is copied once per module. Fine for one or two modules; measured on the largest corpus dump.
