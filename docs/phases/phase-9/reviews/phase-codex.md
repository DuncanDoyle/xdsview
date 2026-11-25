# Phase review — Phase 9 (Codex)

**Scope:** 40822d0..c3fd3fb (code; goldens sampled)
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-25

Reviewed `40822d0..c3fd3fb` on `duncandoyle/phase9`, covering 9.1, 9.2, 9.4 and 9.3. I sampled the 00-simple text and JSON goldens and the network ECDS fixture.

- **P1 · architecture · Snellius can expose config whose policy provenance is incomplete.** Any route-level `PolicyRecord` clears the gap for all per-filter config on that route. A readable CORS policy can therefore mask unattributed ext-auth config. `Dependencies` can return empty `Gaps` even though it cannot enumerate all inputs, defeating R6 and Snellius’s withholding rule. [index.go:240](pkg/envoy/provenance/index.go:240) · [index.go:262](pkg/envoy/provenance/index.go:262)
- **P1 · correctness · A pointer assertion bypasses link validation.** `*FilterLink` implements the exported `Assertion` interface, but `Compose` checks for the value type. A downstream module can submit a pointer with a nonexistent record or a slot on another HCM. It survives composition and serializes as a normal link, violating R4 and P-010 rule 4. [compose.go:67](pkg/envoy/provenance/compose.go:67) · [claim.go:290](pkg/envoy/provenance/claim.go:290)
- **P1 · correctness · ECDS can attach a cluster’s config to a router slot.** Router and cluster upstream filters share Envoy’s upstream ECDS manager. The parser counts only listener subscriptions. If a cluster and router subscribe to the same name and type from different sources, and only the cluster’s entry appears, the router slot is reported resolved with the cluster’s config. The 9.4 record acknowledges this scope limit, which still violates P-010 rule 3. [parser.go:295](pkg/envoy/parser/parser.go:295) · [parser.go:730](pkg/envoy/parser/parser.go:730)
- **P1 · correctness · Policy claim order can vary between runs.** `merge.*` keys are read from a map and records are sorted by `Ref.String()`, which omits the group. Two groups with the same kind, namespace, name and field tie. Their claim IDs and link refs can swap between parses, violating R7. [module.go:200](pkg/adapter/kgateway/module.go:200) · [module.go:222](pkg/adapter/kgateway/module.go:222) · [ref.go:13](pkg/envoy/model/ref.go:13)
- **P1 · correctness · Pruned JSON can retain a link after dropping its record.** A retained route’s policy record can cite evidence on a pruned occurrence. Projection drops the record, but retains a link whose own evidence and slot survive. Its `(module, record)` reference then fails to resolve. The projection test checks the slot but omits the record reference. [output.go:163](cmd/krp/output.go:163) · [main_test.go:204](cmd/krp/main_test.go:204)
- **P2 · architecture · A new assertion kind breaks existing typed JSON readers.** `Claim.UnmarshalJSON` returns an error for an unknown kind. Adding a claim kind under the same schema version can make an older Go consumer reject the entire document, undermining P-010 rule 7. [claim.go:328](pkg/envoy/provenance/claim.go:328)
- **P2 · correctness · The kgwctl row-warning amendment is incomplete.** `render.Text` never reads `snap.Warnings` and draws explicit filter-chain headings without passing their subject to `Style`. A consumer using the public renderer loses subject-scoped warnings such as `hcm-skipped` unless it implements its own display. [render.go:70](pkg/envoy/render/render.go:70) · [render.go:218](pkg/envoy/render/render.go:218) · [design.md:329](docs/phases/phase-9/design.md:329)
- **P2 · correctness · Two plan checks remain open.** The completion record says the required kind capture for scoped `merge.*` was skipped. Phase status still lists the close benchmark, which calls for time and memory measurements for parse, execution and composition on 5,000 routes. [phase-9.3-complete.md:34](docs/phases/phase-9/phase-9.3-complete.md:34) · [implementation-plan.md:79](docs/phases/phase-9/implementation-plan.md:79)
- **P3 · correctness · The dropped-claim gap test passes with the behavior removed.** It rejects the only `RouteSource` on R1, so the ordinary missing-source path produces the same gap even if `Dependencies` ignores dropped claims. [dependencies_test.go:135](pkg/envoy/provenance/dependencies_test.go:135) · [dependencies_test.go:151](pkg/envoy/provenance/dependencies_test.go:151)

No files were edited. Test, vet, dependency, and parity results are from the author.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | Route-level record clears every slot's policyRecord gap | architecture | accept: fixed in b418ebc |
| 2 | Pointer assertion bypasses link validation | correctness | accept: fixed in bde78b2 |
| 3 | ECDS can attach a cluster's config to a router slot | correctness | accept: fixed in 6e3290b |
| 4 | Policy claim order can vary between runs | correctness | accept: fixed in e66f3af |
| 5 | Pruned JSON keeps a link whose record was dropped | correctness | accept: fixed in ef1e36e |
| 6 | Unknown assertion kind breaks older typed readers | architecture | accept: fixed in 927ed5e |
| 7 | `render.Text` ignores subject warnings and chain subjects | correctness | accept: fixed in ad06d96 |
| 8 | Close benchmark and kind capture open | correctness | accept: benchmark in 66cb81c; kind capture deferred to the owner (R5 tested on hand-built fixtures, design Risks) |
| 9 | Dropped-claim gap test passes without the behaviour | correctness | accept: fixed in 5fc4ed4 |