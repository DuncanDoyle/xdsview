# Review — D-023 wrapping module (Codex)

**Scope:** commit 9982c48 (D-023, doc comments on `Claim`, `Module`, `kgateway.ModuleName`, `TestCompose_WrappedModule`)
**Reviewer:** codex (default model), read-only
**Date:** 2026-11-27
**Cross-model:** yes (author anthropic)

Local file links made relative; reviewer text otherwise unchanged.

- **P2 — [pkg/envoy/provenance/claim.go:20](pkg/envoy/provenance/claim.go:20)** — `Module` identifies the outer module passed to `Compose`, so a forwarded kgateway claim gets the wrapper’s name. `Index.Record` also uses `Module` with the target `Ref` as its lookup key. Consumers can ignore module names when choosing claims, but must preserve them for link resolution. D-023 uses the same misleading wording. [compose.go:64](pkg/envoy/provenance/compose.go:64), [index.go:50](pkg/envoy/provenance/index.go:50)
- **P2 — [pkg/envoy/provenance/compose.go:15](pkg/envoy/provenance/compose.go:15)** — A wrapper must keep the target record for its link to survive. Rule 3 reserves only non-empty Refs, after the other claim checks, so a wrapper can reuse a Ref from a claim it omits. Rule 4 also checks route scope, slot, per-filter config, and competing links. Finding the right Ref alone does not guarantee a kept link. [compose.go:66](pkg/envoy/provenance/compose.go:66), [compose.go:264](pkg/envoy/provenance/compose.go:264)
- **P2 — [docs/decisions.md:188](docs/decisions.md:188)** — kgateway emits one record per valid policy ID and field. A `merge.*` key can yield several records or none. A wrapper searching by key or policy kind alone can pick the wrong record. [module.go:225](pkg/adapter/kgateway/module.go:225)
- **P2 — [docs/decisions.md:196](docs/decisions.md:196)** — JSON keeps both projected copies, but text groups identical inherited policies and filter labels by policy and field. The duplicate text claim holds for route-local policy notes only. [output.go:180](cmd/krp/output.go:180), [text.go:54](pkg/envoy/provenance/text.go:54), [text.go:90](pkg/envoy/provenance/text.go:90)
- **P3 — [cmd/krp/claims_test.go:703](cmd/krp/claims_test.go:703)** — Dropping either WAF link alone fails the count, and keeping an extra `AcmePolicy` copy fails the record count or warning check. Replacing the original with a copy under a fresh Ref and linking to it passes. The test checks only the resolved name and subject, and counts links globally. Compare each expected route’s link and resolved Ref with kgateway’s original record.
- **P4 — [cmd/krp/claims_test.go:699](cmd/krp/claims_test.go:699)** — The `/F0` check depends on fixture order. Moving the WAF filter can break the test while the wrapper still finds the correct slot by type.

`ModuleName`’s selection advice matches the implementation. IDs use `<module>/<zero-based emission index>` before validation, so gaps are expected and stability depends on keeping the same name and position. `Record` resolves kept links by `(Module, Ref)` and relies on rule 4 for the target type. Test and vet passes are author-reported.
## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | `Module` must be kept as is: links join by module and Ref | architecture | accept: `Claim` comment and D-023 say so |
| 2 | Finding the Ref isn't enough: keep the record, pass rule 4, don't reuse a dropped claim's Ref | correctness | accept: `Module` comment and D-023 say so |
| 3 | A `merge.*` entry can give several records or none | correctness | accept: D-023 says find by subject, policy, field and level |
| 4 | Text groups inherited duplicates; only route-local notes repeat | correctness | accept: D-023 alternative reworded |
| 5 | Test passes if the wrapper links a fresh copy | correctness | accept: test compares each route's resolved Ref with kgateway's own record; checked that the copy variant fails |
| 6 | `/F0` depends on fixture order | correctness | accept: slots found by filter type |
