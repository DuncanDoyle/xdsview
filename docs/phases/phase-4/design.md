# Phase 4 — Library Extraction: Design

**Roadmap:** `docs/roadmap.md` § Phase 4
**Base:** `e1c5666`
**Unblocks:** program edges E1 (kgwctl Phase 3), E4 (kgateway-ui Phase 9), E6 (move into kgateway)

## Goal

krp's Envoy view becomes an importable Go library at `github.com/DuncanDoyle/krp/pkg/envoy/...`, tagged `v0.1.0`. The krp CLI keeps its flags and text output, and gains `-o json`.

## Non-goals

- Envoy → Gateway API mapping and the plugin seam (Phase 5, P-001/P-002).
- Parser accuracy issues #11, #13, #15, #17 (Phase 5).
- A controller `/snapshots/xds` source (Phase 6).
- A module path change or move into kgateway (E6 happens later).
- API stability beyond v0.x: consumers pin a tag, and breaking changes bump the minor.

## Package layout

```
pkg/envoy/model        types only; JSON tags are the wire contract       stdlib
pkg/envoy/parser       io.Reader → model                                 stdlib + model
pkg/envoy/filter       narrow a snapshot to one HTTPRoute (and rule)     stdlib + model
pkg/envoy/source       Source interface; File and Admin (HTTP) sources   stdlib
pkg/envoy/source/kube  port-forward sources (Gateway, Deployment)        client-go
internal/renderer, internal/tui   unchanged, still internal
cmd/krp                wiring only
```

The core (`model`, `parser`, `filter`, `source`) uses only the standard library. kgateway-ui's core may not import client-go (UI D-016), so the Kubernetes transport sits in its own subpackage. A test lists each core package's dependencies and fails on any outside the standard library and krp's own packages (D-004).

## API

```go
// pkg/envoy/parser — Result replaces ParseResult (no stutter).
type Result struct {
    Snapshot *model.EnvoySnapshot
    Warnings []string // non-fatal: skipped sections
}
func Parse(r io.Reader) (Result, error)

// pkg/envoy/filter — Options replaces FilterOptions.
const AllRules = -1
type Options struct {
    HTTPRouteName, HTTPRouteNamespace string
    RuleIndex int // AllRules or a zero-based index
}
func Filter(s *model.EnvoySnapshot, opts Options) *model.EnvoySnapshot

// pkg/envoy/source — one way to get a config dump; the caller closes the reader.
type Source interface {
    ConfigDump(ctx context.Context) (io.ReadCloser, error)
}
func File(path string) Source
func Admin(baseURL string) Source // GET <baseURL>/config_dump, honours ctx

// pkg/envoy/source/kube — port-forward to the admin port, fetch, close the tunnel.
type Options struct {
    RESTConfig  *rest.Config // used when set; consumers pass their own
    KubeContext string       // otherwise build from kubeconfig with this context
}
func Gateway(name, namespace string, opts Options) source.Source
func Deployment(name, namespace string, opts Options) source.Source
```

The Kubernetes sources open the tunnel inside `ConfigDump` and close it when the returned reader is closed. If anything fails after the tunnel opens (a request error, a non-200 response, a cancelled ctx), `ConfigDump` closes the tunnel before it returns the error. Callers never handle a tunnel. kgwctl and the UI pass the `*rest.Config` they already have, so krp doesn't read kubeconfig a second time with other flags.

`Parse` reads the whole input and then decodes it, so the behaviour stays the same: trailing data after the JSON document is an error, and so is a reader error.

**Ownership.** A snapshot from `Parse` belongs to the caller. `Filter` returns a new snapshot that shares nested values (slices, maps, pointers) with its input. Both are read-only by contract, and the package docs say so. Later phases add derived data, such as Phase 5's Gateway API mapping, as separate structures keyed by Envoy name rather than by mutating a snapshot. That keeps cached and filtered views of one snapshot consistent.

`model` moves unchanged. `TypedConfig`, `TypedPerFilterConfig` and `Metadata` stay `map[string]any`: they are raw Envoy JSON, and typing them is Phase 5 work.

## `-o json` (D-005)

`krp dump -o text|json`, default `text`. `-o json` with `--interactive` is an error. stdout gets one document:

```json
{
  "schemaVersion": "v1alpha1",
  "snapshot": { "listeners": [ ... ] },
  "warnings": [ "..." ]
}
```

`snapshot` is the filtered model, serialised with its JSON tags. `warnings` and `snapshot.listeners` are always arrays, empty when there is nothing (a filter that matches no route gives `"listeners": []`). Warnings still go to stderr as well. `schemaVersion` changes when a field is removed or renamed; adding a field doesn't change it.

Golden files under `testdata/golden/` hold the JSON for each scenario dump. They are the contract consumers test against. `go test ./... -update` rewrites them.

## Test corpus (D-003)

Every existing dump was recorded on Solo Enterprise for kgateway (every one contains `io.solo.*` filters). Scenarios 02_1, 02_6, 02_7 and 02_8 also use enterprise policy APIs.

| Corpus | Location | Source |
|---|---|---|
| OSS (main) | `testdata/scenarios/<s>/envoy/kgateway-v2.4.7/config_dump.json` and `.../kgateway-v2.5.0/...` | re-recorded on kind |
| Enterprise (kept) | `testdata/enterprise/scenarios/<s>/` | existing dumps and manifests, moved as-is |

- The OSS manifests of 02_1 (transformation), 02_6 (CORS), 02_7 (ext auth) and 02_8 (rate limit) are rewritten to OSS `TrafficPolicy` and `GatewayExtension`. Ext auth and rate limit only need a backing Service to exist, because Envoy's filter config doesn't depend on a reachable server. Scenario 06 (multiple ext_proc filters) has a dump but no manifests, so it gets new OSS manifests: two ext_proc `GatewayExtension`s and their `TrafficPolicy`s.
- A recording script builds a kind cluster with its own kubeconfig file, which every `kubectl`/`helm` call receives through `--kubeconfig` together with `--context`. It installs the given kgateway version with Helm. The scenario scripts, including the TLS secret helpers, take kubeconfig and context as arguments.
- Each scenario is recorded in isolation. Setup runs, then a wait until its HTTPRoute and policies report `Accepted`. After that, `/config_dump` is polled until it holds a route for the scenario's HTTPRoute and two reads in a row are equal. Only then is the dump written, and teardown runs before the next scenario starts.
- The parser e2e and filter e2e tests run over every dump in both corpora. Each scenario has a table of expected route names derived from its manifests. The filter tests assert the exact set that is kept, with and without `--rule`, and that other HTTPRoutes' routes are dropped. Passing on v2.4.7 and v2.5.0 confirms the route naming convention still holds.
- `testdata/enterprise/README.md` states that the folder is removed before the move into kgateway (E6).

## Compatibility

- The CLI's flags, defaults, exit codes and text output stay the same. Before the move, the plan captures `krp dump` text output for every scenario. After the move, the output must be byte-identical.
- client-go goes to v0.36.2 (kgateway-ui uses it) and the `go` directive to 1.26.0, which client-go v0.36 requires.
- An Apache-2.0 `LICENSE` is added. Direct dependencies are MIT (charmbracelet) and Apache-2.0 (cobra, k8s.io).

## Risks

| Risk | Mitigation |
|---|---|
| v2.5.0 changes the route naming convention | The filter e2e test on v2.5.0 fails visibly. Fix it in this phase if it's small, otherwise take it to Phase 5 with a finding. |
| OSS `TrafficPolicy` can't express a scenario | Record what OSS can do, and note the gap in the scenario README. |
| Golden JSON churns with every parser change in Phase 5 | Expected. A changed golden file in a diff is a contract change for reviewers to look at. |

## Done when

1. The `pkg/envoy/*` packages exist with Go doc comments, and `internal/{model,parser,filter,envoy}` are gone.
2. The import-boundary test passes.
3. CLI text output is byte-identical to the base for all scenarios.
4. `-o json` works and golden files cover every dump.
5. The OSS corpus is recorded on v2.4.7 and v2.5.0, and the enterprise corpus is moved. Tests pass on both.
6. `LICENSE` is added and client-go is at v0.36.2.
7. A test program in a separate module imports `pkg/envoy/...` through a `replace` directive and builds, which proves the API is usable from outside.
8. Phase close tags `v0.1.0`, updates the program file, and sends cross-repo notices to kgwctl (E1) and kgateway-ui (E4).
