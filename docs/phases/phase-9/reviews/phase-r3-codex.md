# Phase re-review 3 — Phase 9 (Codex)

**Scope:** 8980896..1e30644 fixes
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-25

Local file links and vendor names redacted; reviewer text otherwise unchanged.

### Phase 9 re-review

| Triage | Status | Result |
|---|---|---|
| r1 | Resolved | Route, vhost, and route-config gaps require a qualifying link on each route checked. |
| r7 | Resolved | Upstream warnings render without a route, and compact summaries pass the slot ID to `Style`. |
| r8 | Not resolved | The scoped `merge.*` kind capture remains deferred to the owner. `docs/phases/phase-9/phase-9.3-complete.md:143` |
| listener-gap | Partly resolved | A record for another slot can no longer close the gap. A sibling route’s link to the same slot still can, even though placement can depend on that sibling’s per-filter config. `pkg/envoy/provenance/index.go:246` · `pkg/adapter/kgateway/module.go:251` |
| N1 | Partly resolved | A source with a matching installed slot now gets a gap. An unused source still exposes per-filter config with no gap because the check runs only through HCM uses. `pkg/envoy/provenance/index.go:260` · `pkg/envoy/provenance/index.go:297` |
| N2 | Resolved | Both the link and its record must be inferred or exact. |
| N3 | Partly resolved | The unchecked conversion is fixed. Rejecting a foreign wrapper that embeds a nil `*FilterLink` can still panic when `drop` calls its promoted `Kind()`. `pkg/envoy/provenance/compose.go:144` |
| N4 | Resolved | The design and plan both document the router-upstream ECDS exception. |

### New finding

- **P1 · correctness · Source blocks omit evidence used to close their gaps.** For `Dependencies("RD0")`, route-scoped links can now satisfy the source’s placement check. The check stores only a boolean, and object collection includes in-scope claims plus listener-gap closers. If a source-placement link cites an additional object, that object is omitted and Snellius can show the block to a viewer who cannot read all its evidence. `pkg/envoy/provenance/index.go:141` · `pkg/envoy/provenance/index.go:245` · `pkg/envoy/provenance/index.go:264`

**Listener-gap judgment.** Keeping the gap when no route has per-filter config is sound for R6 and acceptable for Phase 9. The cost is that Snellius can withhold every route block under that HCM, even for a viewer who can read every known contributor. A later additive listener-scoped link can recover that visibility if it identifies the exact slot occurrence, carries listener-level placement evidence, and includes its contributing objects in `Dependencies`. `docs/phases/phase-9/design.md:225` · `docs/phases/phase-9/design.md:235`

Static review only. The author reports passing tests and vet. I verified the diff contains no golden changes and edited no files.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| r8 | Kind capture for scoped `merge.*` still open | correctness | defer: kind capture to the owner |
| listener-gap | Sibling route's link closes a default-on slot's listener gap | correctness | accept: fixed in dc71756 |
| N1 | Unused source exposes per-filter config with no gap | correctness | accept: fixed in 81c2210 |
| N3 | Dropping a wrapper with a nil `*FilterLink` panics in `Kind()` | correctness | accept: fixed in ec7622f |
| new P1 | Source blocks omit evidence used to close their gaps | correctness | accept: fixed in 0617da9 |
| judgment | Listener gap kept when no route has per-filter config | architecture | accept: design §5 note |