# Phase re-review 4 — Phase 9 (Codex)

**Scope:** 1e30644..c19e12e fixes
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-25

**R6 still has P1 blockers.** The six round-3 triage items check out as follows.

| Item | Status | Result |
|---|---|---|
| r8 | Not resolved | Scoped `merge.*` kind capture remains deferred to the owner. [Completion record](docs/phases/phase-9/phase-9.3-complete.md:143) |
| listener-gap | Resolved | Each checked route needs its own qualifying link to the exact slot. [index.go](pkg/envoy/provenance/index.go:254) |
| N1 | Resolved | An unused source with per-filter config gets a route-config policy gap, even if it has a record. [index.go](pkg/envoy/provenance/index.go:337) |
| N3 | Resolved | Drop warnings and gaps identify foreign assertions by Go type without calling their methods. [compose.go](pkg/envoy/provenance/compose.go:144) · [index.go](pkg/envoy/provenance/index.go:347) |
| Round-3 P1 | Resolved | Evidence objects from gap-closing links and records enter `Dependencies.Objects`, including claims outside the block’s scope. [index.go](pkg/envoy/provenance/index.go:144) · [index.go](pkg/envoy/provenance/index.go:275) |
| Listener-gap judgment | Resolved | §5 records the fail-closed behavior when routes have no per-filter config and leaves listener-scoped links for an additive change. [design.md](docs/phases/phase-9/design.md:235) |

**New findings**

- **P2 · correctness.** The unused-source fix treats an Envoy-reported static copy as unused. The parser links that copy through `Reported`, and pruning counts it as used. `Dependencies` gives the static block an unconditional gap even when the identical inline config has complete placement evidence, so Snellius withholds the static block. [index.go](pkg/envoy/provenance/index.go:337) · [parser.go](pkg/envoy/parser/parser.go:586) · [prune.go](pkg/envoy/model/prune.go:39)
- **P3 · architecture.** The new listener check rebuilds and clones the HCM’s full route list for each default-on slot on every route-block query. Checking all route blocks now adds quadratic route-list work and allocations for a large HCM. [index.go](pkg/envoy/provenance/index.go:254) · [index.go](pkg/envoy/provenance/index.go:302)

**Pre-existing R6 blockers**

- **P1 · correctness.** Route blocks render router upstream filters, but dependency scope adds only downstream slots. A retained upstream `ExtensionSource` can name an unreadable GatewayExtension while the route has empty gaps and omits that object. [compose.go](pkg/envoy/provenance/compose.go:195) · [index.go](pkg/envoy/provenance/index.go:191) · [render.go](pkg/envoy/render/render.go:391)
- **P1 · correctness.** The policy-gap scan skips weighted-backend per-filter overrides and source config entries with no matching installed slot. A readable backend or the mere presence of an HCM use can therefore leave `Gaps` empty despite unattributed config in the block. [index.go](pkg/envoy/provenance/index.go:270) · [index.go](pkg/envoy/provenance/index.go:318) · [index.go](pkg/envoy/provenance/index.go:337)

Static review only. The author reports passing `go test ./...` and `go vet ./...`. The diff contains no golden changes, and I edited no files.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| r8 | Kind capture for scoped `merge.*` still open | correctness | defer: kind capture to the owner |
| P2 | Reported static copy treated as an unused source | correctness | accept: fixed in 73a74e4 (fail-closed rework) |
| P3 | Listener check rebuilds route lists per query, quadratic over route blocks | architecture | accept: fixed in 73a74e4 (fail-closed rework) |
| P1 upstream | Upstream slot's `ExtensionSource` outside a route block's scope | correctness | accept: fixed in 73a74e4 (fail-closed rework) |
| P1 policy scan | Weighted-backend overrides and slot-less source entries skipped | correctness | accept: fixed in 73a74e4 (fail-closed rework) |