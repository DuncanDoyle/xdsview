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
| 4 | ECDS and wrapper state | A filter with `config_discovery` takes its config from the `EcdsConfigDump` entry whose name is the filter's name and whose type is in `type_urls` (`ConfigSource: ecds`). A router upstream filter is the exception: a cluster's upstream filter shares its subscriptions and clusters aren't parsed, so a matching entry could be the cluster's. Its slot stays unresolved with an `ecds-ambiguous` warning. With no entry, on an active listener with `default_config` set, the default is in use: Envoy applies it when discovery fails, and `apply_default_config_without_warming` only decides whether it waits (`ecdsDefault`). Otherwise the filter stays in place: `Runs` unknown, `State` unresolved. Composite and ExtensionWithMatcher set `Wrapper`; both set `Conditional`. |
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
| `RouteSource` | route | HTTPRoute ref, rule index, rule name, match index, index certainty, incomplete | alternatives | as read |
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
func (ix *Index) Inputs(block model.ID) []Input
func (ix *Index) Warnings() []model.Warning

type Resolution[T Assertion] struct {
	Certainty  model.Certainty // ambiguous when Candidates disagree
	Candidates []T
	Claims     []Claim
}

type Dependencies struct {
	Objects []Dependent // every object the block's claims rest on, with the claim kind, sorted
	Gaps    []Gap       // expected attribution that is missing or ambiguous
}

type Gap struct {
	Subject model.ID
	Kind    string   // the assertion kind expected, e.g. "routeSource"
	Slot    model.ID // policyRecord only: the HTTP filter slot, empty when none is installed
	Level   Level    // policyRecord only: the level the record is missing at, empty for a weighted backend
	Cluster string   // backendSource only: the mirror cluster, empty for the route's own backend
}
```

`Dependencies(block)` is fail-closed (design review #1, Codex phase re-reviews 1–4). It first enumerates every input the block shows or relies on, then checks each input against the claim kinds that can cover it. Every uncovered input is a gap; there is no per-case gap rule.

**1. Inputs.**
- Every listener, filter chain, virtual host, route and weighted backend in or above the block, and every mirror cluster of such a route.
- Every network filter other than the HCM in a filter chain in or above the block.
- Every HTTP filter slot, downstream and router upstream, of every HCM serving the block: the HCM in or above it, or for a route config source block, every HCM whose use resolves to it.
- Every per-filter config entry at route config level (the source a use in or above the block resolves to, inline or RDS, or the source block itself), and at virtual host, route and weighted backend level in or above the block. An entry with no matching installed slot is an input too.
- A static source that an inline source records as `Reported` is the same source as that inline config: its block has the inline source's inputs, the inline source's listener lineage included, not those of an unused source. A source with no use is its own block of inputs: its per-filter config entries.

**Bootstrap.** A static listener (`Listener.Static`), everything in it, its inline route configs and their `Reported` static copies come from the proxy's startup config, not from Kubernetes, so no claim can cover them. They are still inputs, with status `bootstrap`: never a gap, and they add no objects. Dynamic (xDS) listeners are unaffected.

**2. Cover.** An input is covered only by claims at inferred or exact. A filter link covers only on the route being checked; the routes checked are the block's routes beneath the input, or all of them when the block holds none.

| Input | Covered by |
|---|---|
| listener | `GatewaySource` |
| filter chain, HCM, use | nothing expected |
| virtual host | `HostSource` |
| route | `RouteSource` |
| route or weighted backend with a cluster | `BackendSource` |
| mirror cluster; route whose cluster is picked per request (header, specifier plugin) | nothing: always a gap |
| network filter other than the HCM | nothing: always a gap |
| HTTP filter slot, downstream or upstream, other than the router | `ExtensionSource` |
| downstream slot that runs by default, not the router, not marked a helper | also: for each route checked, a kept `FilterLink` on that route placing a listener-level `PolicyRecord` on the slot |
| per-filter config entry for slot S at level X (route, virtual host, route config) | for each route checked, a kept `FilterLink` on that route placing a level-X `PolicyRecord` on S |
| per-filter config entry on a weighted backend, or with no installed downstream slot | nothing: always a gap |

Alternatives kinds cover when they resolve at inferred or exact (`Resolve`), so ambiguous readings are gaps. A policy gap names the slot and level (`Gap.Slot`, `Gap.Level`). A readable record for one filter never covers another filter's entry. A default-on slot whose routes have no per-filter config keeps its listener-level gap in Phase 9; a listener-scoped link is the additive fix later (rule 7). Without `merge.*` at a level (U1), a hidden policy could add config there; the gap makes that visible. Router upstream slots need only `ExtensionSource`: rule 4 admits no link to them. A route's `BackendSource` is its primary cluster, so a mirror has no subject a claim could cover without making it ambiguous; its gap names it (`Gap.Cluster`). No Phase 9 kind has a network filter as subject, and a filter such as RBAC or `tcp_proxy` changes what the route serves, so it is a gap rather than Envoy-only infrastructure. Each can gain a claim kind later (rule 7).

**3. Objects.** `Objects` holds the evidence objects of every claim on an input's subject, on the route config sources in scope and on the serving HCMs' slots, inherited policy records included. Every claim that covers an input adds its evidence objects too, wherever its subject lives.

**Inputs.** `Dependencies` flattens evidence and gap causes. A consumer visibility strategy needs them per input, so the index also lists the inputs themselves ([visibility-strategies-codex.md](reviews/visibility-strategies-codex.md) §1):

```go
func (ix *Index) Inputs(block model.ID) []Input // sorted like Gaps

type Input struct {
	Gap                     // subject, expected kind, slot, level, cluster
	Status     InputStatus  // covered, gap, bootstrap
	Reason     GapReason    // gap only
	Candidates []Candidate  // routeSource only: every RouteSource claim on the route
	Complete   bool         // routeSource only: the readings at the highest certainty name every possible source
}

type Candidate struct {
	Ref   model.Ref // the HTTPRoute the claim names
	Claim Claim     // with its evidence and certainty
}
```

| Reason | Meaning |
|---|---|
| `ambiguous` | several candidates at the highest certainty |
| `noEvidence` | no claim at inferred or better |
| `dropped` | Compose dropped a claim of the kind on the subject (rule 3) |
| `incomplete` | the route's readings may miss its source: a reading at the highest certainty is marked `Incomplete` |
| `notCoverable` | no claim kind can cover it: a network filter, a mirror, a per-request cluster, a per-filter entry with no installed downstream slot |

A route's candidate list is complete (`Complete`) when its highest-certainty readings are exact, or are readings no module marks `Incomplete` (Codex visibility-strategies review, P1). A module marks a reading `Incomplete` when it could not enumerate every reading, such as a route name outside the expected format. An empty or incomplete list is a gap, never covered: `noEvidence` when empty, otherwise `incomplete`, which wins over `ambiguous`.

`Dependencies` derives from the same enumeration: its `Gaps` are the inputs with status `gap`. A route's candidates are listed whether or not their objects exist in any cluster: xdsview never consults cluster state, and the consumer's strategy decides what a missing candidate means.

**4. Cost.** The index builds its per-HCM structure and the link placements once, on the first call. A route block's inputs are its lineage and its HCM's slots, so checking every route block is linear in routes.

kgwctl's route lookup is `Resolve[RouteSource](ix, routeID)`. Snellius withholds a block when the viewer can't read every object in `Dependencies(block).Objects`, or when `Gaps` is not empty (R6).

```go
type Dependent struct {
	Object model.Ref
	Kind   string // assertion kind, e.g. "backendSource": consumers treat kinds differently
}
```

**Gaps** (Snellius review, snellius#70). An empty `Gaps` means every input is covered. Two notes on the rule above:
1. `dropped`: a claim in scope that Compose rule 3 dropped also gives its subject a gap of the dropped claim's kind.
2. `gatewaySource`: a consumer that knows the Gateway it captured may close a listener's gap itself, as its own context. Listeners from a ListenerSet are not attributed in Phase 9: they show this gap.

Two tests check the rule on the corpus, for every dump and fixture and every listener, virtual host, route and route config source block. An oracle walks krp's JSON output, independently of the index, and requires an input for every item the block shows: each cluster (primary, weighted, mirror, per-request selector), network filter, HTTP slot, per-filter config entry, listener, virtual host and route. And removing any single claim that covers an input from the kgateway module's output adds that input's own gap. A third test checks, for every such block, that `Dependencies(block).Gaps` equals the gap-status inputs of `Inputs(block)`.

### 6. kgateway module (R5, R8, R9, R13)

`kgateway.New() provenance.Module`. It takes no caller context. `KgatewayOptions.Gateway` is gone: a consumer that knows which Gateway it captured shows that itself, labelled as its own context. `Known` moves to selection.

- Today's mapper logic becomes claim producers. Each claim lists its evidence: route name (`name`), `route_source` metadata (`metadata`), the field table (`table`).
- `merge.*` is decoded at listener, route config and virtual host level, beside route level. Each record is a `PolicyRecord` on its own level. Nothing says which level governs (P-008).
- Field links follow rule 4: one matching slot or none. A Composite's delegates are candidates too (#5). The rustformation check reads the field Envoy loads the module from; when `name` and `module` disagree, there is no link (#4).
- A route name whose rule name contains dots reads its rule index and name (#2).
- A route name is parsed after its own `<vhost>-route-<N>-` prefix, so a marker inside the virtual host name can't change the readings. A name without that prefix is read after every possible prefix, and its readings are marked `Incomplete`. `Names` has no virtual host, so it always reads after every possible prefix.
- The policy-field table names the kgateway version it was checked against. Its check: every field in the corpus's `merge.*` records is in the table or a recorded exclusion (R14, #30).

Selection (R8):

```go
func Select(ix *provenance.Index, snap *model.Snapshot, q Query) Selection

type Query struct {
	Route     model.Ref
	RuleIndex *int // nil: any rule. With RuleName, both must match
	RuleName  string
	Known     []model.Ref // HTTPRoutes that exist; narrows readings (rules below)
}

type Selection struct {
	Routes     []model.ID  // one reading, matching the query, not narrowed by Known
	Ambiguous  []model.ID  // several readings, or one left only by Known; at least one matches
	Unresolved []model.ID  // a route-name marker whose text could name the queried HTTPRoute and rule index (where set), but no valid reading
	Narrowed   []Narrowing // every route Known narrowed, whatever its bucket
}

type Narrowing struct {
	Route   model.ID
	Kept    []provenance.Claim // RouteSource claims Known kept
	Dropped []provenance.Claim // RouteSource claims Known removed
}

// Narrow applies Known to every route's readings, without a query.
func Narrow(ix *provenance.Index, known []model.Ref) []Narrowing
```

Selection rules (kgwctl review, kgwctl#14):
1. `Query.Route` is an HTTPRoute (`gateway.networking.k8s.io`). Other route kinds are additive later.
2. A reading matches when its HTTPRoute equals `Query.Route`, and the rule index and rule name match where set.
3. `Routes`: the route's `RouteSource` resolves to one reading at inferred or exact, it matches, and Known did not narrow it.
4. `Ambiguous`: several readings and at least one matches; or one reading left only because Known removed the others (kgwctl `view.go:169`). Ambiguous routes are never selected.
5. Known removes a reading only when its HTTPRoute is absent from Known. It never removes an exact reading, and never removes the last one: when every reading is outside Known, Known is ignored for that route and no narrowing is recorded.
6. Certainty is never raised by Known, and there is no `narrowed` certainty: a narrowed route keeps `ambiguous`, and its `Narrowing` names what was kept.

krp's live mode without `--route` calls `Narrow` and prints the kept reading inline, marked as narrowed, as today.

Consumers prune with `model.Prune(snap, in(sel.Routes))`. The index keeps every candidate.

Names (R9): `kgateway.Names() provenance.NameMapper`. It takes no options and names the kgateway minors its name rules were checked against (R14); a consumer that supports several minors checks its minor against that list. Every ambiguous answer lists every candidate the name can read as, never a subset: Snellius hides a name unless the viewer can read all of them.

```go
type NameMapper interface {
	Name(kind NameKind, name string) NameAnswer // kind: cluster, routeConfig, listener
}

type NameAnswer struct {
	Certainty  model.Certainty
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


**kgwctl review additions** (kgwctl#14):
1. `provenance.Labels(ix, narrowings)` returns the on-row origin text, including a narrowed reading inline; `Notes(ix)` keeps the lines below rows.
2. `Assertion` has `Kind() string`. In JSON a claim's assertion is an object with `kind` plus its fields; the schema doc lists each kind.
3. `Snapshot` has typed lookups by ID (`Listener`, `VirtualHost`, `Route`, `Slot`, each returning the item and a bool). Every virtual host and route that comes from a route config source has `SourcePath` (`RD3/V2/R5`), equal across the uses of a shared source, so a consumer can list it once.
4. Order: claims are sorted by subject with `model.CompareID`, which compares index segments numerically (`L2` before `L10`), then module order, then emission order. Candidates keep the module's reading order.
5. `Row.Subject` is the listener, filter chain, vhost, route or slot ID of the printed row. `render.Text` prints a subject's warnings under its row and one coverage line for ignored sections; dump-wide warnings stay with the caller.
6. `ix.Claims()` returns all claims. `provenance.Records(ix, snap, routeID)` returns the `PolicyRecord`s at every level that applies to the route, each with its level.
7. `Conditional` has two meanings by level, both kept: `EffectiveChain.Conditional` means a weighted backend may override per-filter config; `ChainEntry.Conditional` means the entry's matcher decides.
8. Warning codes are exported constants in `model`. `NameAnswer` has no separate `Kind`: each candidate `Ref` carries it.


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
