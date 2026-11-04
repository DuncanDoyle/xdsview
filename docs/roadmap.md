# krp — Roadmap

**Last updated:** 2026-11-04
Program: [kgateway-tooling `program.md`](https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md) (local checkout: `../kgateway-tooling-claude`)

## Goal

`krp` (Kgateway Route Printer) visualizes the Envoy configuration that kgateway programs for a Kubernetes Gateway API deployment: listeners, filter chains, HCM, routes and the HTTP filter pipeline, optionally narrowed to one HTTPRoute.

krp is the **Envoy data-plane library** of the kgateway tooling program. Its packages are consumed by:
- a new kgateway CLI (first consumer, `proxy routes`-style commands);
- kgateway-ui Phase 9 "Runtime Filter Chain".

The `krp` binary stays as the library's reference CLI/TUI and test bed. The Gateway API resource graph, policy attachment and status errors belong to kgateway-ui and the CLI, not to krp (program decisions P-001 to P-005).

The project follows the agentic-engineering-sdlc workflow (`docs/phase-status.md`, `docs/phases/phase-<N>/`, `docs/decisions.md`).

---

## Phases

### Phase 1 — Envoy Config Viewer
**Status:** Complete (patch cycles 1.1–1.4 applied)
**Docs:** `docs/phases/phase-1/`

CLI that parses a raw Envoy config dump (from file or live port-forward) and renders the complete structure: listeners, network-level filter chains, HCM, RDS route configs, virtual hosts, routes, HTTP filter pipeline, and backend clusters. No K8S awareness. Static TUI output.

### Phase 2 — HTTPRoute Selector
**Status:** Complete
**Docs:** `docs/phases/phase-2/`

Add K8S awareness to select a specific HTTPRoute and optionally a rule index. Uses the deterministic route naming convention (`httproute-<name>-<ns>`) embedded in Envoy route names to filter the view down to only the relevant listeners/routes/filters for that HTTPRoute.

### Phase 3 — Filter Config Detail
**Status:** Complete (sub-phases 3.1–3.5 applied)
**Docs:** `docs/phases/phase-3/`

Interactive mode (`--interactive`/`-i`): navigate filters, expand/collapse typed config per filter or all at once. Typed config sources: `typed_per_filter_config` on route entries and filter-level config in the HCM `http_filters` array.

---

### Phase 4 — Library Extraction
**Status:** Complete (2026-11-04, `v0.1.0`) — `docs/phases/phase-4/phase-4-complete.md`
**Docs:** `docs/phases/phase-4/`
**Unblocks:** program edges E1 (CLI), E4 (kgateway-ui Phase 9), E6 (move into kgateway)

Turn krp's core into an importable, stable Go library without changing CLI behaviour.

- Move `model`, `parser` and `filter` from `internal/` to `pkg/envoy/`. They must not import client-go, lipgloss or bubbletea.
- Parser accepts an `io.Reader`; consumers supply the transport.
- `pkg/envoy/source`: how to get a dump — file and port-forward (client-go) — behind one interface, so an admin-endpoint source can be added later.
- `-o json` output of the (filtered) model, which doubles as the contract consumers test against.
- Add an Apache-2.0 `LICENSE`.
- Bump client-go to the version kgateway-ui uses (v0.36).
- Re-record test config dumps on kgateway 2.4 and 2.5; verify the route naming convention still holds.
- Tag `v0.1.0` and update the program file.

**Packages (after):** `pkg/envoy/{model,parser,filter,source}`; `internal/{renderer,tui}` stay internal.

### Phase 5 — Data-Plane Accuracy and Mapping
**Status:** Not started
**Docs:** `docs/phases/phase-5/`
**Unblocks:** program edge E5

Make the model faithful to what Envoy runs, and own the mapping back to Gateway API (program decision P-001).

- Fix the kube source leaving a port-forward connection open when a call is cancelled mid-connect (Phase 4 Q9). Needed before kgateway-ui uses the kube source in its long-running backend.
- Close #11 (`prefix_rewrite`), #13 (`weighted_clusters`), #15 (HTTPRoute filters), #17 (multi-route test dumps); consider #16.
- Multi-listener and TLS scenarios (`03-multi-policy`, `04-multi-rule`, `05-multi-listener`).
- Envoy name → Gateway API ref mapping (Gateway, listener, HTTPRoute, rule) as a library function, behind an interface so a consumer inside kgateway can use kgateway's own naming code. kgateway-ui's `computed.EnvoyNamer` delegates to it.
- Policy provenance from Envoy metadata (e.g. `filter_metadata` merge keys) where present, through a plugin seam. Vendor-specific policy kinds live outside the core (program decision P-002).

### Phase 6 — Intended vs Accepted Configuration
**Status:** Not started
**Docs:** `docs/phases/phase-6/`
**Unblocks:** program edge E4 (kgateway-ui Phase 9, second half)

Compare the controller's intended xDS (kgateway admin `/snapshots/xds`) with what Envoy accepted (`/config_dump`), and report differences per route and filter. Covers the "Config Mismatches" TODO: a filter the controller produced that Envoy does not run, or the reverse.

- New source for the controller snapshot.
- Diff model (added / missing / changed) exposed by the library and rendered by the `krp` CLI.

---

## Deferred / out of roadmap

Ideas not on the phased roadmap, kept for later. Content formerly in `docs/plans/possible-future-phases.md` and `docs/plans/TODO.md`.

### Deferred: Envoy ↔ K8S Correlation

Partly planned as Phase 5 (Envoy → Gateway API mapping). Correlate Envoy filters and routes back to originating K8S resources (Gateway, HTTPRoute, TrafficPolicy, EnterpriseKgatewayTrafficPolicy, etc.). Uses a layered matching strategy:
1. kgateway metadata in Envoy config (`filter_metadata`, `typed_per_filter_config` references)
2. Structural matching (VirtualHost domains, route match config, cluster naming `kube_<ns>_<svc>_<port>`)
3. Route name conventions (embeds HTTPRoute name/namespace)

Annotates each filter in the Envoy TUI with its K8S source resource, providing a bridge between the low-level Envoy view (Phases 1–3) and the K8S resource view (Phases 4–7).

---

### Deferred: Envoy Side-by-Side Detail View

Side-by-side Envoy config + K8S manifest view when selecting a filter in the Envoy TUI. Search, `--json` output, and UX refinements to the existing interactive mode.

---

### Superseded: Gateway API Track (former Phases 4–7)

Removed from the krp roadmap on 2026-11-04. kgateway-ui (Phases 2–5) and the planned kgateway CLI cover this ground; see the kgateway-tooling program file. Kept as a record of the original intent.

- **Gateway API DAG Printer (`krp graph`)** — GatewayClass → Gateway → HTTPRoute tree with rules, matches, backend refs, HTTPRoute filters and listener bindings, from a live cluster or a YAML directory.
- **Policy Support** — Gateway API native policies, kgateway policies and vendor policies attached via `ExtensionRef` / `targetRefs`, as plugin-provided node types.
- **Status Detection and Error Marking** — mark nodes whose conditions report errors, and flag resources with no status at all (controller has not processed them).
- **Interactive DAG Detail View** — Phase 3-style expand/collapse of node specs.

---

### Future Considerations

- **REST API / server mode:** `RouteGraph` model designed for JSON serialization. Adding `--serve` flag for a Web UI backend requires only a thin HTTP handler.
- **Config mismatch detection:** Surface K8S policies that exist but are absent from the Envoy config (translator bugs, xDS errors). Partly planned as roadmap Phase 6 (intended vs accepted).
- **Global Policy Namespace:** Support policy attachment from a designated global namespace to targets in other namespaces.

### From the TODO list

#### Global Policy Namespace
Kgateway has a feature called "Global Policy Namespace". This feature allows the user to define Kgateway policies (e.g. TrafficPolicy, EnterpriseKgatewayTrafficPolicy, etc.) in a different namespace than its target. Normally, in K8S Gateway API, a `targetRef` is a LocalObjectReference, meaning that the policy needs to be deployed in the same namespace as its target. "Global Policy Namespace" allows you to select a single namespace on your K8S cluster from which policies can attach to any target.

Currently, I don't think `krp` is able to detect global policy attachment from policies deployed in the "Global Policy Namespace" to targets in other namespaces.


#### Config Mismatches
Instead of just presenting the filterchain that is present in Envoy, and linking the filter configuration to K8S Gateway API, we should also be able to detect if an expected filter is not in the filterchain. This should be a very uncommon scenario, but it could occur in the case of bug in the translator, or an error in xDS that therefore is not loaded in the Gateway.

---

## Architecture

```
pkg/envoy/source  ──►  pkg/envoy/parser  ──►  pkg/envoy/model  ──►  pkg/envoy/filter
 (file, port-forward,      (io.Reader →           (JSON-serialisable)    (HTTPRoute narrowing,
  controller snapshot)      model)                                        Envoy → Gateway API mapping)
                                                        │
                       ┌────────────────────────────────┼─────────────────────────┐
                       ▼                                ▼                         ▼
             krp CLI / TUI (internal/)          kgateway CLI               kgateway-ui backend
```

---

## Test Scenarios

Two corpora (D-003). The OSS corpus in `testdata/scenarios/` is recorded with `hack/record-dumps.sh <version>` on OSS kgateway v2.4.7 and v2.5.0, into `envoy/kgateway-<version>/`. The enterprise corpus in `testdata/enterprise/scenarios/` holds the earlier recordings from the enterprise distribution (06 from Gloo Gateway v1). It's kept for parser coverage and removed before the move into kgateway.

| Scenario | Description | OSS v2.4.7 / v2.5.0 | Enterprise |
|----------|-------------|:---:|:---:|
| 00-simple | HTTP Gateway, 1 route, no policies | Yes | Yes |
| 01_1–01_9-matchers | Path (prefix, exact, regex), header, method and other matchers | Yes | Yes |
| 02_1-single-policy | HTTPS Gateway (2 hostnames), transformation policy | Yes | Yes |
| 02_2-single-policy | HTTP, RequestHeaderModifier (native K8S filter) | Yes | Yes |
| 02_3-single-policy | HTTP, ResponseHeaderModifier (native K8S filter) | Yes | Yes |
| 02_4-single-policy | HTTP, RequestMirror (native K8S filter) | Yes | Yes |
| 02_5-single-policy | HTTP, URLRewrite (native K8S filter) | Yes | Yes |
| 02_6-single-policy | HTTP, CORS | Yes | Yes |
| 02_7-single-policy | HTTP, ExtAuth | Yes | Yes |
| 02_8-single-policy | HTTP, RateLimit (global) | Yes | Yes |
| 03-multi-policy | Multiple policies on one route | No (gateway only) | No |
| 04-multi-rule | One route, multiple rules/backends | No (gateway only) | No |
| 05-multi-listener | Gateway with HTTP + HTTPS listeners | No (gateway only) | No |
| 06-multi-extproc | Two HTTPRoutes, one ext_proc extension each | Yes | Yes (Gloo Gateway v1) |
