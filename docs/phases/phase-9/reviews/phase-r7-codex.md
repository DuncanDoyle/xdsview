# Phase re-review 7 — Phase 9 (Codex)

**Scope:** 77215bd..b0cd997
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-26

Local file links and vendor names redacted; reviewer text otherwise unchanged.

1. **Partly resolved.** Static listeners now expose RDS attribution gaps, but `kgateway.New()` still skips their fetched routes and RDS source metadata. Valid evidence can go unused, even when a dynamic listener shares the source. `pkg/adapter/kgateway/module.go:43` · `pkg/adapter/kgateway/module.go:60`
2. **Resolved.** Every highest-certainty reading must affirm `Exhaustive` and clear `Incomplete`.
3. **Resolved.** Filter rows retain policy levels and drop the override label.
4. **Resolved.** A dropped mirror claim now sets `Gap.Cluster` to the mirror.

**New findings**

- **P1 · correctness.** `Resolve` compares whole assertions, so two modules that agree on the route and indices but differ only on `Exhaustive` produce a false ambiguity. Text can display `ambiguous:` followed by a single route. `pkg/envoy/provenance/index.go:106` · `pkg/envoy/provenance/text.go:222`
- **P2 · correctness.** Each bootstrap check under a static listener scans all fetched uses. With one route per static RDS use, checking every route block becomes quadratic, violating §5’s linear-cost contract. `pkg/envoy/provenance/index.go:336` · `docs/phases/phase-9/design.md:284`

Static review. Tests and vet passed per the author.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | The kgateway module skips route config a static listener fetches over RDS (partly resolved) | correctness | accept: fixed in 031873f |
| 2 | `Complete` must be affirmed by every highest-certainty reading (resolved) | architecture | no action |
| 3 | Filter rows keep policy levels without the override label (resolved) | correctness | no action |
| 4 | A dropped mirror claim names the mirror cluster (resolved) | correctness | no action |
| P1 | `Resolve` makes readings that differ only in `Exhaustive` ambiguous | correctness | accept: fixed in fe554fb |
| P2 | The bootstrap check under a static listener makes `Dependencies` quadratic | correctness | accept: fixed in db013d1 |
