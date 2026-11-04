# krp — Project Roadmap

**Date:** 2026-03-08
**Last updated:** 2026-11-04
**Program:** [kgateway-tooling `program.md`](https://github.com/DuncanDoyle/kgateway-tooling/blob/main/program.md) (local checkout: `../kgateway-tooling`)

## Overview

`krp` (Kgateway Route Printer) visualizes the Envoy configuration that kgateway programs for a Kubernetes Gateway API deployment: listeners, filter chains, HCM, routes and the HTTP filter pipeline, optionally narrowed to one HTTPRoute.

krp is the **Envoy data-plane library** of the kgateway tooling program. Its packages are consumed by:
- a new kgateway CLI (first consumer, `proxy routes`-style commands);
- kgateway-ui Phase 9 "Runtime Filter Chain".

The `krp` binary stays as the library's reference CLI/TUI and test bed. The Gateway API resource graph, policy attachment and status errors belong to kgateway-ui and the CLI, not to krp (program decisions P-001 to P-005).

Each phase has its own design and implementation document pair in `docs/plans/`. See `docs/plans/possible-future-phases.md` for deferred and speculative work.

---

## Phases

### Phase 1 — Envoy Config Viewer
**Status:** Complete (patch cycles 1.1–1.4 applied)
**Docs:** `phase-1-envoy-viewer-design.md`, `phase-1-envoy-viewer-implementation.md`

CLI that parses a raw Envoy config dump (from file or live port-forward) and renders the complete structure: listeners, network-level filter chains, HCM, RDS route configs, virtual hosts, routes, HTTP filter pipeline, and backend clusters. No K8S awareness. Static TUI output.

### Phase 2 — HTTPRoute Selector
**Status:** Complete
**Docs:** `2026-03-17-phase-2-httproute-selector-design.md`, `2026-03-17-phase-2-httproute-selector-implementation.md`

Add K8S awareness to select a specific HTTPRoute and optionally a rule index. Uses the deterministic route naming convention (`httproute-<name>-<ns>`) embedded in Envoy route names to filter the view down to only the relevant listeners/routes/filters for that HTTPRoute.

### Phase 3 — Filter Config Detail
**Status:** Complete (sub-phases 3.1–3.5 applied)
**Docs:** `2026-03-21-phase-3-filter-config-detail-design.md`, `2026-03-21-phase-3-filter-config-detail-implementation.md`, `phase-3-complete.md`

Interactive mode (`--interactive`/`-i`): navigate filters, expand/collapse typed config per filter or all at once. Typed config sources: `typed_per_filter_config` on route entries and filter-level config in the HCM `http_filters` array.

---

### Phase 4 — Library Extraction
**Status:** Not started
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
**Unblocks:** program edge E5

Make the model faithful to what Envoy runs, and own the mapping back to Gateway API (program decision P-001).

- Close #11 (`prefix_rewrite`), #13 (`weighted_clusters`), #15 (HTTPRoute filters), #17 (multi-route test dumps); consider #16.
- Multi-listener and TLS scenarios (`03-multi-policy`, `04-multi-rule`, `05-multi-listener`).
- Envoy name → Gateway API ref mapping (Gateway, listener, HTTPRoute, rule) as a library function, behind an interface so a consumer inside kgateway can use kgateway's own naming code. kgateway-ui's `computed.EnvoyNamer` delegates to it.
- Policy provenance from Envoy metadata (e.g. `filter_metadata` merge keys) where present, through a plugin seam. Vendor-specific policy kinds live outside the core (program decision P-002).

### Phase 6 — Intended vs Accepted Configuration
**Status:** Not started
**Unblocks:** program edge E4 (kgateway-ui Phase 9, second half)

Compare the controller's intended xDS (kgateway admin `/snapshots/xds`) with what Envoy accepted (`/config_dump`), and report differences per route and filter. Covers the "Config Mismatches" TODO: a filter the controller produced that Envoy does not run, or the reverse.

- New source for the controller snapshot.
- Diff model (added / missing / changed) exposed by the library and rendered by the `krp` CLI.

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

Real K8S + Envoy config pairs in `testdata/scenarios/`:

| Scenario | Description | Has config_dump |
|----------|-------------|:---:|
| 00-simple | HTTP Gateway, 1 route, no policies | Yes |
| 01_1–01_9-matchers | Path (prefix, exact, regex), header, method and other matchers | Yes |
| 02_1-single-policy | HTTPS Gateway, transformation policy | Yes |
| 02_2-single-policy | HTTP, RequestHeaderModifier (native K8S filter) | Yes |
| 02_3-single-policy | HTTP, ResponseHeaderModifier (native K8S filter) | Yes |
| 02_4-single-policy | HTTP, RequestMirror (native K8S filter) | Yes |
| 02_5-single-policy | HTTP, URLRewrite (native K8S filter) | Yes |
| 02_6-single-policy | HTTP, CORS | Yes |
| 02_7-single-policy | HTTP, ExtAuth | Yes |
| 02_8-single-policy | HTTP, RateLimit | Yes |
| 03-multi-policy | Multiple policies on one route | No |
| 04-multi-rule | One route, multiple rules/backends | No |
| 05-multi-listener | Gateway with HTTP + HTTPS listeners | No |
| 06-multi-extproc | Multiple ext_proc filters | Yes |
