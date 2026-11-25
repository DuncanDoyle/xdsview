# Phase 9 — Control-Plane-Agnostic Contract: design

**Status:** draft (2026-11-25), for owner review, then Codex review, then kgwctl and Snellius review
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
| R14 | Version-keyed tables (sides, policy field to filter) name the Envoy or kgateway version they were checked against, and have a check. | P-012 rule 4, 6 |

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
| core | `pkg/envoy/source`, `source/kube` | unchanged | as today |
| contract | `pkg/envoy/provenance` | `Claim`, assertion kinds, `Module`, `Compose`, `Index`, `NameMapper` | model |
| module | `pkg/adapter/kgateway` | today's `origin/kgateway.go` and `filter`: claims, `merge.*` and `route_source` decoding, route selection, names | model, provenance |

`origin` and `filter` are removed. A dependency test fails the build if `model`, `parser`, `chain` or `render` import `provenance` or `adapter/...` (extends the D-004 test).

### 2. Snapshot and occurrence IDs (R3, R11)

`parser.Parse(r) (*model.Snapshot, error)`. Warnings move into the snapshot, typed and keyed by ID.

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

IDs are assigned once, in `parser.Parse`, as structural paths: `L0`, `L0/C1`, `L0/C1/N0` (a network filter), `L0/C1/N0/F3` (an HTTP filter slot), `L0/C1/N0/U0` (upstream filter), `L0/C1/N0/RC` (the HCM's route config use), `.../RC/V2`, `.../RC/V2/R5`, `.../R5/W1`. Route config sources get `RD3` (dynamic) or `RS0` (static). Consumers compare IDs and call `Within`; they never parse them.

Every type that has an ID carries it as a field. `model.Prune(snap, keep func(ID) bool)` returns a copy that keeps the selected routes and their ancestors, and every non-active listener (today's rule). IDs survive pruning, so any result computed on the full snapshot still joins (Codex #4).

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
	Conditional   bool   // runs only when its matcher matches
}
```

`RequestOrder` and `ResponseOrder` keep entries whose run state or side is `unknown`, as today for sides. The sides table stays in `chain` (modules never supply sides, rule 3) and names its Envoy version (R14). Its check: a test fails when a filter type in any fixture is missing from the table without a recorded exclusion.

### 4. Core correctness (R12)

Each item changes OSS output. Goldens change only through reviewed `-update` runs.

| # | Item | Behaviour |
|---|---|---|
| 1 | Missing RDS (E14) | An active listener's HCM with an RDS name and no matching source gets `RouteConfigState` `missing` and a `rds-missing` warning. It is `unknown` when the dump is filtered, `RoutesConfigDump` is absent, or any route config entry was skipped. Warming and draining listeners stay `unknown`. |
| 2 | Static listeners and route configs | `static_listeners` and `static_route_configs` are parsed. `Listener.Static` marks them. An inline `route_config` on the HCM is a source too. |
| 3 | Non-HCM chains | Every filter chain lists its network filters (ID, name, type). A chain without an HCM is kept, not dropped. |
| 4 | ECDS and wrapper state | A filter with `config_discovery` takes its config from `EcdsConfigDump` when present. Otherwise it stays in place: `Runs` unknown, `State` unresolved. Composite and ExtensionWithMatcher set `Wrapper`; ExtensionWithMatcher sets `Conditional`. |
| 5 | Matchers | Header and query matchers read every `string_match` form, `present_match`, `range_match`, `safe_regex`, `invert_match` and `ignore_case`. `RouteMatch.CaseSensitive` is read. Predicates the parser doesn't model (gRPC, runtime fraction, dynamic metadata, TLS context, path match policy) are listed in `RouteMatch.Unparsed`, never dropped. |
| 6 | Default filter chain | `Listener.DefaultFilterChain` is parsed with its own ID (`L0/Cd`). |
| 7 | Cluster selectors | `Route.Action.ClusterHeader` and `ClusterSpecifierPlugin` are read. The route has no cluster name, and says why. |
| 8 | Route actions | `Route.Action.Kind`: `route`, `redirect`, `directResponse`, `filterAction`, `nonForwarding`. Redirect and direct response keep their fields (status, location parts; body size, never the body). |
| 9 | Coverage policy | `Snapshot.Coverage` lists every config dump section: `parsed`, `partial` (entries skipped), or `ignored` (clusters, endpoints, secrets, scoped routes, and unknown types). Secrets are never decoded. An HCM with `scoped_routes` gets `RouteConfigState` `unknown`. Text output prints one line for ignored sections. |

### 5. Provenance contract (R2, R4, R6, R7)

```go
// Claim is one assertion a module makes about one occurrence.
type Claim struct {
	ID        string          // stable within one Compose result
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

func Compose(snap *model.Snapshot, modules ...Module) *Index
```

`Assertion` is a sealed interface; new kinds are additive (rule 7). Phase 9 kinds:

| Kind | Subject | Fields | Certainty cap |
|---|---|---|---|
| `ListenerSource` | listener, filter chain, route config use | Gateway (optional), port, section | as read |
| `HostSource` | virtual host | listener source, hostname | as read |
| `RouteSource` | route | HTTPRoute ref, rule index, rule name, match index, index certainty | as read |
| `ExtensionSource` | HTTP filter slot | extension ref, filter kind, helper | as read |
| `PolicyRecord` | listener, route config use, virtual host, route | policy ref, field, level | exact (validated `merge.*`) |
| `FilterLink` | route | record claim ID, slot ID | inferred, only when one slot matches |
| `BackendSource` | route or weighted backend | backend ref, port | as read |

Compose rules:
1. Each module gets its own deep copy of `snap`. A test runs every module and checks the original is unchanged (R2).
2. A claim whose subject is not in `snap`, or that exceeds its kind's cap, is dropped with a warning on the index. Caller context is not evidence (rule 5): a claim with no evidence is dropped.
3. Claims keep module order, then emission order. `Claim.ID` is `<module>/<n>`.
4. For one subject and kind, the highest certainty wins. Several distinct values at that certainty resolve as ambiguous; all are kept.

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
	Objects      []model.Ref // every object any claim inside block rests on, sorted
	Unattributed []model.ID  // routes and slots inside block with no claim above unknown
}
```

kgwctl's route lookup is `Resolve[RouteSource](ix, routeID)`; its unresolved listing is the routes whose resolution is below inferred. Snellius withholds a block when the viewer can't read every object in `Dependencies(block).Objects`, or when `Unattributed` is not empty (R6).

### 6. kgateway module (R5, R8, R9, R13)

`kgateway.New(opts) provenance.Module`. `Options.Gateway` stays, used only to fill the Gateway in `ListenerSource` claims that rest on name evidence; it is labelled caller context, never evidence by itself. `Options.Known` moves to selection.

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
	Routes    []model.ID  // selected
	Ambiguous []model.ID  // could match, not selected
	Narrowed  []Narrowing // a reading dropped because its HTTPRoute isn't in Known
}
```

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
```

### 7. Rendering and krp (R10)

`render.Text(w, snap, chains, opts)`. The hooks take IDs instead of index tuples: `Style func(Class, model.ID, string) string` and `Notes func(model.ID) []string`. `provenance.Notes(ix)` returns a `Notes` function with today's origin, policy and contribution lines, so krp and kgwctl print the same text.

krp flow: parse → `chain.Effective` → `provenance.Compose(snap, kgateway.New(...))` → `kgateway.Select` when `--route` is set → `model.Prune` → render. The TUI reads the same three results.

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
