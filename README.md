# xdsview

`xdsview` is a Go library that fetches, parses and explains the Envoy config of a [Kgateway](https://github.com/kgateway-dev/kgateway) proxy: it labels Envoy objects with the Gateway API objects that produced them. Import it from `github.com/DuncanDoyle/xdsview/pkg/envoy/...`.

The repo also ships `krp` (Kgateway Route Printer), a CLI built on the library. `krp` is a CLI tool that visualizes the [Envoy](https://www.envoyproxy.io/) route configuration for [Kgateway](https://github.com/kgateway-dev/kgateway) API Gateway deployments.

Kgateway translates Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/) resources (Gateway, HTTPRoute, TrafficPolicy, etc.) into Envoy configuration. Understanding the exact filter chain that processes your traffic — including filter ordering, applied policies, and backend routing — is difficult without digging through raw JSON config dumps. `krp` makes this visible.

> **Status:** Active development — Phases 1–6 complete. See [docs/roadmap.md](docs/roadmap.md).

---

## Features

**Phase 1 (complete) — Envoy Config Viewer**
- Parse a live or file-based Envoy config dump
- Visualize the complete configuration: listeners, filter chains, HTTP filter pipeline, virtual hosts, routes, and backend clusters
- Auto port-forward to the gateway-proxy pod (no manual setup needed)

**Phase 2 (complete) — HTTPRoute Selector**
- Filter the rendered output to routes belonging to a specific HTTPRoute (`--route`, `--route-ns`)
- Optionally narrow to a single rule within the HTTPRoute (`--rule`, by index or name)
- Label routes, filters and policies with the Gateway API objects that produced them
- Port-forward to any Deployment pod via `--deployment` (alternative to `--gateway`)

**Planned**
- Phase 3: Expand individual filters to see their typed configuration
- Phase 4: Correlate Envoy filters back to the originating K8S Gateway API resources (Gateway, HTTPRoute, TrafficPolicy, EKTP)
- Phase 5: Interactive detail view with side-by-side Envoy config + K8S manifest

---

## Installation

> **Note:** The module is not yet published. Install by building from source.

Build from source:

```bash
git clone https://github.com/DuncanDoyle/xdsview.git
cd xdsview
go build -o krp ./cmd/krp
```

---

## Usage

### Visualize from a config dump file

```bash
krp dump --file path/to/config_dump.json
```

### Visualize from a live cluster

```bash
# krp automatically port-forwards to the gateway-proxy pod
krp dump --gateway gw -n kgateway-system

# Port-forward to any pod in a Deployment
krp dump --deployment gw -n kgateway-system

# With an explicit kubeconfig context
krp dump --gateway gw -n kgateway-system --context my-cluster
```

### Filter by HTTPRoute

```bash
# Show only routes for a specific HTTPRoute
krp dump --file config_dump.json --route my-route --route-ns default

# Narrow to a single rule within the HTTPRoute: a zero-based index or a rule name
krp dump --file config_dump.json --route my-route --route-ns default --rule 0
krp dump --file config_dump.json --route my-route --route-ns default --rule api
```

### Where it came from

`krp dump` labels Envoy objects with the Gateway API objects that produced them:
- each route's HTTPRoute, rule and match
- each policy filter's `GatewayExtension`
- each route's policies, from kgateway's `merge.*` route metadata

```
└─ route / (prefix) · HTTPRoute kgateway-system/api-example-com · rule 0 · match 0 (chosen from known HTTPRoutes)
   filters (request order):
     1 ext_auth/kgateway-system/ext-authz  enabled by route · GatewayExtension kgateway-system/ext-authz (inferred) · TrafficPolicy kgateway-system/ext-auth (extAuth, inferred)
     2 envoy.filters.http.router           on
   not run: global_disable/ext_auth (off)
   policies: TrafficPolicy kgateway-system/ext-auth (extAuth)
```

A route name can be read several ways, because HTTPRoute names and namespaces may both contain `-`. Three things settle it:

1. In live mode (`--gateway`, `--deployment`), krp lists the cluster's HTTPRoutes.
2. `--route`/`--route-ns` names one. It takes the place of the listing.
3. kgateway runs with `enableRouteSourceMetadata: true` (Helm), which writes each route's source into Envoy's config. kgateway marks it experimental.

A route settled by 1 or 2 is marked "(chosen from known HTTPRoutes)": a route Envoy still serves for a deleted HTTPRoute can resolve to a live one. With `--route`, krp shows such a route when the one reading left is the named HTTPRoute and rule; `-o json` marks it `"selected": true` under `selection.narrowed`.

With `--file` and none of these, a route prints `ambiguous:` and its readings. `-o json` lists every reading as a `routeSource` claim, with a `certainty` of `exact`, `inferred` or `ambiguous`.

Every label states how it was derived. `(inferred)` marks one read from a naming convention or the kgateway module's field-to-filter table; `exact` ones, read from metadata, print bare. `(partly from name)` marks a route whose identity came from metadata while at least one of its rule index, rule name or match index came from the Envoy route name. A policy is linked to a filter only when exactly one filter configured for the route matches its field; otherwise it shows under `policies:` only. Policies recorded at the virtual host, route config or listener show under `inherited policies:`.

With `--gateway`, listener lines end with `Gateway <ns>/<name> (from --gateway)`: the Gateway krp was pointed at, not something read from the dump.

### Effective filter chain

Each route lists the HCM filters that run for it, in request order, and one `not run:` line for the rest. kgateway disables policy filters in the HCM and enables them per route, so the HCM list alone doesn't say what a route runs. For each filter, the most specific `typed_per_filter_config` entry decides: route, then virtual host, then route config.

| HCM `disabled` | Deciding entry | Runs | State |
|---|---|---|---|
| false | none | yes | `on` |
| false | `FilterConfig{disabled: true}` | no | `disabled` |
| false | `FilterConfig{config: …}` or a filter-specific config | yes | `on` |
| true | none | no | `off` |
| true | `FilterConfig{disabled: true}` | no | `off` |
| true | `FilterConfig{config: …}` or a filter-specific config | yes | `enabled` |
| any | `FilterConfig` with neither field (Envoy rejects it) | no | `unknown` |

A weighted-cluster backend can override the route's entry when Envoy picks it. krp then prints the chain as the "baseline before backend selection" and names the backends that override. Response order is the reverse of the filters with a response side. Which filters have one comes from a table cited from the Envoy source (`pkg/envoy/chain/sides.go`). A filter type that isn't in the table is marked `?`. The TUI (`-i`) lists every filter inline, so each one can be selected and expanded.

### JSON output

```bash
# Print the (filtered) snapshot as JSON; composes with every source and filter flag
krp dump --file config_dump.json -o json
```

The document (`schemaVersion` `v1alpha2`) has these fields:

| Field | Content |
|---|---|
| `snapshot` | The parsed dump: `listeners` (always an array), `routeConfigs` (the route config sources each HCM's `routeConfigUse` points at) and `coverage`. Every occurrence has an `id`, valid only within that document. |
| `chains` | Each route's effective filter chain by route ID, and an HCM's chain by its `routeConfigUse` ID when no route config joined. Each entry names its HCM filter `slot`; `runs`, `request` and `response` are `yes`, `no` or `unknown`. |
| `claims` | What the kgateway module says each occurrence came from: `id`, `subject`, `assertion` (an object with `kind` and its fields), `certainty`, `evidence` and `module`. Kinds: `listenerSource`, `gatewaySource`, `hostSource`, `routeSource`, `extensionSource`, `backendSource`, `policyRecord` (with its `level`) and `filterLink` (a `policyRecord`'s `record` ref and the `slot` it configures on the claim's route). |
| `selection` | With `--route`: the `query`, the `routes` krp shows, the `ambiguous` and `unresolved` routes it leaves out, and the `narrowed` routes with the readings kept and dropped. In live mode without `--route`: `narrowed` only. Absent otherwise. |
| `warnings` | Every warning, typed: `subject` (absent for a dump-wide one), `code` and `message`. Parse warnings and dropped claims alike. |

With `--route`, every field except `selection` covers only what `snapshot` keeps: no ID outside `selection` is absent from `snapshot`. `selection` describes the whole dump, so its entries may name routes `snapshot` lacks; they carry the route `name` and `virtualHost` beside the `id`. `schemaVersion` changes only when a field is removed or renamed. `testdata/golden/` holds the expected output for every test dump. `-o json` can't be combined with `--interactive`.

### Collecting a config dump manually

```bash
kubectl port-forward -n kgateway-system deploy/gw 19000:19000 &
sleep 2
curl -s localhost:19000/config_dump | jq . > config_dump.json
kill %1
```

---

## Requirements

### Live cluster mode (`--gateway` / `--deployment`)

- A running Kubernetes cluster with Kgateway installed
- RBAC: `get pods` and `create pods/portforward` in the Gateway namespace
- Optional RBAC: `list httproutes.gateway.networking.k8s.io` in all namespaces, for route origins. Without it krp warns, and routes whose names read several ways print without their HTTPRoute.
- Your kubeconfig context pointing at the target cluster

### File mode (`--file`)

No cluster access required — works fully offline.

---

## Example Output

`testdata/golden/scenarios/05-multi-listener/kgateway-v2.5.0.txt`, an HTTP and an HTTPS listener (krp adds colour):

```
listener~80 [::]:80 · active · port 80
└─ vhost listener~80~api_example_com [api.example.com] · port 80
   └─ route / (prefix) · HTTPRoute kgateway-system/api-example-com · rule 0 · match 0 (chosen from known HTTPRoutes)
      filters (request order):
        1 envoy.filters.http.router  on
      backend: kube_httpbin_httpbin_8000
listener~443 [::]:443 · active · port 443
└─ chain https-api · TLS api.example.com
   └─ vhost https-api~api_example_com [api.example.com] · listener https-api
      └─ route / (prefix) · HTTPRoute kgateway-system/api-example-com · rule 0 · match 0 (chosen from known HTTPRoutes)
         filters (request order):
           1 envoy.filters.http.router  on
         backend: kube_httpbin_httpbin_8000
```

A listener that is warming, draining or rejected shows its state and Envoy's error, e.g. `listener~8443 [::]:8443 · warming · error: <details> (<time>)`.

---

## Project Structure

```
cmd/krp/          CLI entrypoint (cobra)
pkg/envoy/        Importable library (standard library only, except source/kube)
  model/          Snapshot data model
  parser/         Envoy config dump JSON parser (io.Reader)
  chain/          Effective HTTP filter chain per route
  render/         Plain-text tree (what krp prints, without colour)
  provenance/     Claims about where occurrences came from, composed from modules
  source/         Config dump sources: File, Admin (HTTP)
    kube/         Port-forward sources: Gateway, Deployment (client-go)
pkg/adapter/
  kgateway/       The kgateway OSS provenance module, route selection and name mapping
internal/
  renderer/       Terminal output (lipgloss)
  tui/            Interactive TUI (bubbletea)
docs/             Roadmap, phase status, decisions, per-phase docs (docs/phases/)
testdata/
  scenarios/      Real K8S + Envoy config dump pairs for testing
  fixtures/       Hand-built config dumps
  golden/         Expected CLI output
```

### Using xdsview as a library

```go
// Fetch and parse a config dump.
src := kube.Gateway("gw", "kgateway-system", kube.Options{RESTConfig: cfg})
rc, err := src.ConfigDump(ctx)
if err != nil {
	return err
}
defer rc.Close() // also closes the port-forward

// Capture tells the parser whether the dump is known to be complete.
snap, err := parser.Parse(rc, parser.Options{Capture: src.Capture()})
if err != nil {
	return err
}

// Each route's runtime filter chain, keyed by route ID, from Envoy data alone.
chains := chain.Effective(snap)
```

Snapshots are read-only once shared: `model.Prune` returns a view that shares data with its input.

```go
// Claims about where each occurrence came from. A vendor module is another
// provenance.Module; Compose gives each one its own copy of the snapshot.
ix, err := provenance.Compose(snap, kgateway.New(), vendorModule)
if err != nil {
	return err // duplicate module names
}
route := provenance.Resolve[provenance.RouteSource](ix, routeID) // candidates and certainty

// The routes of one HTTPRoute. Known HTTPRoutes narrow names that read
// several ways; a narrowed route stays in Ambiguous with its Narrowing.
q := kgateway.Query{Route: model.Ref{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "shop", Name: "orders"}, Known: httpRoutes}
sel := kgateway.Select(ix, snap, q)
keep := map[model.ID]bool{}
for _, id := range sel.Routes {
	keep[id] = true
}
pruned := model.Prune(snap, func(id model.ID) bool { return keep[id] })

// The tree krp prints, without colour, with the provenance labels and policies lines.
err = render.Text(os.Stdout, pruned, chains, render.Options{
	Header:  "Gateway kgateway-system/gw",
	Labels:  provenance.Labels(ix, sel.Narrowed),
	Trailer: provenance.Notes(ix),
})

// A bare name from proxy stats: a cluster, route config, listener or route.
answer := provenance.Guess(kgateway.Names(), "kube_shop_cart_8080") // Service shop/cart
```

Each chain entry carries its slot ID; `Runs`, `Request` and `Response` are `yes`, `no` or `unknown`. `render.Options` has `AllFilters` (list every HCM filter inline), `ResponseOrder`, and hooks keyed by `render.Row` (route ID and slot ID): `Style`, `Labels` (shown on a row after " · "), `Notes` (lines under a route or filter row) and `Trailer` (labelled lines after a route's filters). `ix.Dependencies(block)` lists the objects a block's claims rest on and the attribution it lacks.

---

## Testdata Scenarios

`testdata/scenarios/` holds Kubernetes manifests and Envoy config dumps recorded on OSS kgateway v2.4.7 and v2.5.0 (`envoy/kgateway-<version>/`). `testdata/fixtures/` holds hand-built dumps for config no recording has (router upstream filters). `testdata/golden/` holds the expected CLI output for both. See the Test Scenarios table in [docs/roadmap.md](docs/roadmap.md).

Re-record the OSS corpus on a throwaway kind cluster (needs Docker, kind, Helm, kubectl, jq):

```bash
hack/record-dumps.sh v2.5.0                 # every scenario with an HTTPRoute
hack/record-dumps.sh v2.5.0 02_7-single-policy   # just one
```

Apply a single scenario to a cluster of your own with `hack/scenario.sh up|down <scenario-dir> <kubeconfig> <context>`.

---

## Contributing

This project is in early development. Contributions, bug reports, and feature suggestions are welcome via GitHub Issues.

---

## License

Apache 2.0 — see [LICENSE](LICENSE).
