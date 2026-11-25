# Approach consult — Phase 9 releases and API shape (Codex)

**Scope:** pre-design consult on two decisions; not the required design review.
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok.
**Prompt:** options listed without the author's pick.
**Date:** 2026-11-25

## 1. Releases — choose A, one tag at Phase 9 close

Land the contract break first in code, then ship it with the correctness fixes. E15 specifies one tag, and P-010 puts the fixes in the same phase. B could preserve the new API across both tags, but it would deliver a second round of observable output changes. A delays the first consumable tag until the fixes are ready. [E15](/Users/ddoyle/Development/claude/kgateway-tooling-claude/program.md:47) · [Order of work](/Users/ddoyle/Development/claude/kgateway-tooling-claude/program.md:89) · [P-010](/Users/ddoyle/Development/claude/kgateway-tooling-claude/program.md:198)

- **kgwctl** can migrate `proxy routes` once, replace the embedded snapshot with its own `ProxyRoutes` payload, bump the shared envelope to `kgwctl.kgateway.dev/v1alpha2` across all JSON/YAML commands, and review the route text, JSON and YAML goldens in the same pass. B spreads that review across the schema migration and a later behavior update. [Current payload](/Users/ddoyle/Development/claude/kgwctl-claude-phase4/internal/proxyview/view.go:52) · [Envelope](/Users/ddoyle/Development/claude/kgwctl-claude-phase4/internal/print/envelope.go:14) · [Golden tests](/Users/ddoyle/Development/claude/kgwctl-claude-phase4/internal/app/proxy_routes_test.go:59)
- **Snellius** Phase 12 can target one version and validate runtime display and evidence withholding against the corrected execution model. With B it would either validate twice or wait for the second tag. Phase 12 is still unstarted in this checkout. [Snellius roadmap](/Users/ddoyle/Development/claude/snellius-claude/docs/roadmap.md:185)

## 2. Go API — choose 1, with typed claims and indexed access

- **1. Position-keyed claims.** This fits rules 1, 2, 4–6 and scope 1–4 if input is genuinely immutable or isolated, claims carry per-object evidence, and composition preserves conflicting alternatives. Give a field→filter link both the policy-record ID and the installed-slot ID in its route context. Add scope 5–8 explicitly: enumerable evidence dependencies, consumer-controlled selection with all candidates retained, a separate `NameMapper`, and consumer-owned wire payloads. Rule 3 still requires the core fixes and rule 2 requires the independent-parse invariance test. Typed assertions with indexed lookup make named additions under rule 7 practical. [P-010 rules and scope](/Users/ddoyle/Development/claude/kgateway-tooling-claude/program.md:188)
- **2. Mirrored tree.** It can satisfy rules 1–3 and scope 1–2. To satisfy rules 4–6 and scope 3–5, its typed fields still need per-assertion evidence, conflict sets, occurrence IDs, and cross-node links. Scope 6–8 still need explicit APIs and projections. Rule 7 is possible, but mirroring makes consumers track tree changes and leaves Snellius walking across levels to determine the dependencies of one block. Use a tree as a display projection if helpful. [P-010 scope](/Users/ddoyle/Development/claude/kgateway-tooling-claude/program.md:198)
- **3. Revised `Mapper`.** A separate overlay and deterministic composition can cover scope 1–3 and rules 1, 4 and 6 with appropriate claim types. Value copies still alias maps and slices, so rule 2 needs isolation. The existing callbacks lack occurrence and parent context for scope 4–5 and contextual evidence under rule 5. Supplying those inputs redesigns the seam anyway. Rule 3 remains core work, scope 6–8 remain separate work, and freezing the callback interface makes rule 7 harder. [Mapper](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/origin/origin.go:21) · [Shared copies](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/origin/origin.go:123)

Give kgwctl a route-ID lookup for its payload and unresolved-route listing, and give Snellius a block-level dependency query so it can withhold the whole block when any required evidence is unreadable or its scope is unknown. `krp` should project both results into its own `v1alpha2` envelope. This keeps each consumer from rebuilding the joins. [Snellius visibility check](/Users/ddoyle/Development/claude/snellius-claude/pkg/api/findings.go:191) · [krp envelope](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/cmd/krp/output.go:17)

**Risks in the current code**

- `Effective` is already pure, but `chain.Annotate` invokes the mapper and stores contributions on execution entries. The renderer prefers that annotated chain, and `krp` prunes before annotation through a kgateway-aware filter. Assign IDs before pruning and carry them through every projection. [Annotation](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/chain/annotate.go:24) · [Renderer](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/render/render.go:247) · [krp pre-filter](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/cmd/krp/main.go:226)
- RDS lookup silently overwrites duplicate names and clones a shared config per HCM. Keep the config’s source occurrence and each HCM’s use distinct. kgwctl’s `(routeConfigName, routeName)` dedup still collapses distinct uses. Missing RDS requires a complete dump and an explicit reference on an active HCM. Filtered or incomplete coverage yields unknown. [RDS index](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/parser/parser.go:100) · [RDS attachment](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/parser/parser.go:177) · [kgwctl dedup](/Users/ddoyle/Development/claude/kgwctl-claude-phase4/internal/proxyview/view.go:125) · [E14 finding](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/docs/investigations/xds-convergence.md:107)
- Moving attribution alone leaves the deferred dotted-rule and rustformation bugs. It also leaves an unresolved ECDS slot looking runnable because the chain defaults to `Runs: !f.Disabled`. [Phase 8 deferrals](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/docs/phases/phase-8/phase-8-complete.md:40) · [Execution default](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/chain/chain.go:32)
## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | Releases: A | architecture | accept (owner chose A, 2026-11-24) |
| 2 | API: option 1 with typed claims and indexed lookup | architecture | accept, pending owner |
| 3 | Route-ID lookup for kgwctl, block-dependency query for Snellius | architecture | accept into design |
| 4 | IDs assigned before pruning, carried through every projection | correctness | accept into design |
| 5 | Duplicate RDS names overwritten; shared config cloned per HCM | correctness | accept into design (core correctness) |
| 6 | Unresolved ECDS slot shown as running | correctness | accept into design (P-010 rule 3) |
