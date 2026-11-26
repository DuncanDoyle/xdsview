# Phase re-review — Phase 9 (Codex)

**Scope:** c3fd3fb..8980896 fixes
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-25

Local file links and vendor names redacted; reviewer text otherwise unchanged.

### Findings 1–9

| # | Status | Re-review |
|---|---|---|
| 1 | **Partly resolved** | A CORS link can no longer clear an ext-auth gap. Placement still accepts claims of any certainty and lets a sibling route’s link clear an inherited gap even when placement is ambiguous on the route being checked. |
| 2 | **Resolved** | Pointer `FilterLink` and `PolicyRecord` assertions go through the value-form checks. The regression test covers dangling and cross-HCM links. |
| 3 | **Resolved** | Router upstream ECDS stays unresolved when a matching entry could belong to a cluster. |
| 4 | **Resolved** | Record ordering includes the API group and breaks ties deterministically. |
| 5 | **Resolved** | JSON projection drops a link whenever its referenced record is pruned. |
| 6 | **Resolved** | Future assertion kinds decode as `Opaque` and retain their payload. |
| 7 | **Partly resolved** | Subject warnings and chain IDs reach `Style`. Upstream-slot warnings still disappear when the HCM has no rendered route, and compact filter summaries pass the route’s subject to `Style` for slot warnings. |
| 8 | **Partly resolved** | The 5,000-route benchmark reports time and memory for all three stages. The kind capture remains deferred. |
| 9 | **Resolved** | The dropped `HostSource` is the only possible source of the gap asserted by the test. |

### Listener-level gap

**Yes, it is the same R6 masking problem.** A readable listener policy for CORS can clear the gap on a default-on ext-auth slot whose provenance is unknown. The smallest sound rule is to leave the gap open for every default-on slot subject to the existing router and helper exclusions. Close it only when evidence attributes listener-level config to that exact slot occurrence and includes the contributing objects in `Dependencies`. A route-scoped `FilterLink` establishes per-route placement. `pkg/envoy/provenance/index.go:248`

### New findings

- **P1 · correctness · Source blocks can lose their only gap.** `Dependencies("RD0")` puts no HCM slots in scope, and the new check requires an in-scope slot. A source with per-filter config and zero records can now return empty `Gaps`. The old source-level check caught it. `pkg/envoy/provenance/index.go:163` · `pkg/envoy/provenance/index.go:233`
- **P1 · correctness · Unknown claims can now close a policy gap.** `placed` checks claim survival alone. An `Unknown` record and link pass the composer’s upper-bound checks and can suppress a gap the previous rule kept open. `pkg/envoy/provenance/index.go:224` · `pkg/envoy/provenance/compose.go:227`
- **P1 · correctness · Pointer normalization can panic.** A downstream type can embed an assertion and override `Kind` on a pointer receiver. The pointer implements `Assertion`, but its dereferenced value fails the unchecked type assertion in `byValue`. `pkg/envoy/provenance/compose.go:119`
- **P2 · correctness · The ECDS contract is stale.** Design §4 and the plan still promise a name/type join. The fix leaves matching router-upstream entries unresolved by design. `docs/phases/phase-9/design.md:132` · `pkg/envoy/parser/parser.go:738`

The author reports passing `go test ./...` and `go vet ./...`. I reviewed the diff statically, verified that it contains no golden changes, and edited no files.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| r1 | Sibling route's or unknown-certainty link closes a policy gap | correctness | accept: fixed in 151e554 |
| r7 | Upstream-slot warnings lost without a route; summaries style by route | correctness | accept: fixed in 0e55fba |
| r8 | Kind capture for scoped `merge.*` still open | correctness | defer: kind capture to the owner |
| listener-gap | Listener record for another filter closes a default-on slot's gap | architecture | accept: fixed in f88218a |
| N1 | Source block loses its per-filter config gap | correctness | accept: fixed in b58106f |
| N2 | Unknown claims close a policy gap | correctness | accept: fixed in 151e554 |
| N3 | Pointer normalisation can panic | correctness | accept: fixed in 7bf4fad |
| N4 | ECDS contract stale for router upstream entries | correctness | accept: fixed in 0f8bf46 |