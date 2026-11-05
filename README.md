# krp — Kgateway Route Printer

`krp` is a CLI tool that visualizes the [Envoy](https://www.envoyproxy.io/) route configuration for [Kgateway](https://github.com/kgateway-dev/kgateway) API Gateway deployments.

Kgateway translates Kubernetes [Gateway API](https://gateway-api.sigs.k8s.io/) resources (Gateway, HTTPRoute, TrafficPolicy, etc.) into Envoy configuration. Understanding the exact filter chain that processes your traffic — including filter ordering, applied policies, and backend routing — is difficult without digging through raw JSON config dumps. `krp` makes this visible.

> **Status:** Active development — Phases 1–3 complete; Phase 4 (library extraction) in progress. See [docs/roadmap.md](docs/roadmap.md).

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
git clone https://github.com/DuncanDoyle/krp.git
cd krp
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
└─ Route: / (prefix)
    HTTPRoute kgateway-system/api-example-com · rule 0 · match 0
    HTTP Filters:
    ├─ [1] global_disable/ext_auth (disabled)
    ├─ [2] ext_auth/kgateway-system/ext-authz ← GatewayExtension kgateway-system/ext-authz
    └─ [3] envoy.filters.http.router
    Policies:
    └─ TrafficPolicy kgateway-system/ext-auth (extAuth)
```

A route name can be read several ways, because HTTPRoute names and namespaces may both contain `-`. krp shows a route's HTTPRoute only when there's one reading. Three things settle it:

1. In live mode (`--gateway`, `--deployment`), krp lists the cluster's HTTPRoutes.
2. `--route`/`--route-ns` names one.
3. kgateway runs with `enableRouteSourceMetadata: true` (Helm), which writes each route's source into Envoy's config. kgateway marks it experimental.

With `--file` and none of these, routes print without their HTTPRoute. `-o json` lists every reading, with a `certainty` of `exact`, `inferred` or `ambiguous`.

### JSON output

```bash
# Print the (filtered) snapshot as JSON; composes with every source and filter flag
krp dump --file config_dump.json -o json
```

The document is `{"schemaVersion": "v1alpha1", "snapshot": {...}, "warnings": [...]}`. `warnings` and `snapshot.listeners` are always arrays, never `null`. `schemaVersion` changes only when a field is removed or renamed. `testdata/golden/` holds the expected output for every test dump. `-o json` can't be combined with `--interactive`.

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
- Optional RBAC: `list httproutes.gateway.networking.k8s.io` in all namespaces, for route origins. Without it krp warns and prints routes without their HTTPRoute.
- Your kubeconfig context pointing at the target cluster

### File mode (`--file`)

No cluster access required — works fully offline.

---

## Example Output

```
╭─ Listener: listener~443 ─────────────────────────── [::]:443 ─╮
│                                                                 │
│  ├─ FilterChain[0] https-api  TLS: api.example.com             │
│  │  └─ HCM → RDS: https-api                                    │
│  │     └─ VirtualHost: https-api~api_example_com               │
│  │           [api.example.com]                                  │
│  │        └─ Route: / (prefix)                                  │
│  │           HTTP Filters:                                      │
│  │           ├─ [1] io.solo.transformation (disabled)           │
│  │           └─ [2] envoy.filters.http.router                   │
│  │           Backend: kube_httpbin_httpbin_8000                 │
│                                                                 │
│  └─ FilterChain[1] https-developer  TLS: developer.example.com │
│     └─ HCM → RDS: https-developer                              │
│        └─ VirtualHost: https-developer~developer_example_com   │
│              [developer.example.com]                            │
│           └─ Route: / (prefix)                                  │
│              HTTP Filters:                                      │
│              ├─ [1] io.solo.transformation (disabled)           │
│              └─ [2] envoy.filters.http.router                   │
│              Backend: kube_httpbin_httpbin_8000                 │
╰─────────────────────────────────────────────────────────────────╯
```

---

## Project Structure

```
cmd/krp/          CLI entrypoint (cobra)
pkg/envoy/        Importable library (standard library only, except source/kube)
  model/          EnvoySnapshot data model
  parser/         Envoy config dump JSON parser (io.Reader)
  filter/         HTTPRoute snapshot filter
  origin/         Envoy object → Gateway API object mapping (kgateway OSS, pluggable)
  source/         Config dump sources: File, Admin (HTTP)
    kube/         Port-forward sources: Gateway, Deployment (client-go)
internal/
  renderer/       Terminal output (lipgloss)
  tui/            Interactive TUI (bubbletea)
docs/             Roadmap, phase status, decisions, per-phase docs (docs/phases/)
testdata/
  scenarios/      Real K8S + Envoy config dump pairs for testing
```

### Using krp as a library

```go
// Fetch, parse and narrow a config dump to one HTTPRoute.
rc, err := kube.Gateway("gw", "kgateway-system", kube.Options{RESTConfig: cfg}).ConfigDump(ctx)
if err != nil {
	return err
}
defer rc.Close() // also closes the port-forward

result, err := parser.Parse(rc)
if err != nil {
	return err
}
snap := filter.Filter(result.Snapshot, filter.Options{
	HTTPRouteName: "api-example-com", HTTPRouteNamespace: "default", RuleIndex: filter.AllRules,
})
```

Snapshots are read-only once shared: `filter.Filter` returns a view that shares data with its input.

```go
// Label a snapshot with Gateway API origins. Known HTTPRoutes resolve route
// names that can be read several ways; a vendor plugin is another Mapper in the chain.
m := origin.Chain(vendorMapper, origin.Kgateway(origin.KgatewayOptions{
	Gateway: &model.Ref{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "kgateway-system", Name: "gw"},
	Known:   httpRoutes,
}))
annotated := origin.Annotate(snap, m)

// A bare name from proxy stats: a cluster or a route config.
ref, ok := origin.ObjectForName(m, "kube_shop_cart_8080") // Service shop/cart
```

---

## Testdata Scenarios

`testdata/scenarios/` holds Kubernetes manifests and Envoy config dumps recorded on OSS kgateway v2.4.7 and v2.5.0 (`envoy/kgateway-<version>/`). `testdata/enterprise/` holds older recordings from the enterprise distribution, kept as extra parser coverage. See the Test Scenarios table in [docs/roadmap.md](docs/roadmap.md).

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
