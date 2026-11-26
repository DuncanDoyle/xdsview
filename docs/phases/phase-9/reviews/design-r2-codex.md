# Design + plan re-review — Phase 9 (Codex)

Local file links and vendor names redacted; reviewer text otherwise unchanged.

**Scope:** amendments `ac3cb72..10af6ff` against design-codex.md
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-25

### Original findings

| # | Status | Re-review |
|---|---|---|
| 1 | **Partly resolved** | Route blocks now include inherited claims and per-kind gaps. For listener and filter-chain blocks, the dependency rule still needs to follow every descendant HCM’s route-config source, including shared RDS sources outside the block’s ID subtree. |
| 2 | **Resolved** | Capture completeness is explicit. Offline input defaults to unverified, and the plan covers complete, filtered, and draining cases. |
| 3 | **Resolved** | Row hooks carry route and slot IDs. The plan tests a shared slot with different notes under two routes. |
| 4 | **Resolved** | Design and plan preserve active listeners with warming replacements, draining versions, or rejected updates. |
| 5 | **Partly resolved** | Caller context is removed and policy records are cumulative. Independently evidenced fields within `ListenerSource` still compete as whole values, and Compose checks for evidence presence without checking that it supports each asserted object. |
| 6 | **Resolved** | Module-local refs, duplicate-module rejection, and validation of record, HCM, and applicable config address the original finding. |
| 7 | **Partly resolved** | Source/use identity and dynamic-only lookup are explicit. Tests still omit policy metadata on a source with zero uses and on one shared by multiple uses. |
| 8 | **Partly resolved** | `Narrow` covers live mode without a query. `Unresolved` still needs to be scoped to the queried HTTPRoute and rule. The design should also preserve kgwctl’s ambiguity when `Known` alone leaves one reading. |
| 9 | **Partly resolved** | The design adds matcher-dependent wrappers, missing matcher and action forms, and uncertainty labels. The plan still omits ECDS type and default cases, Composite conditional behavior, CONNECT matching, and the inline cluster plugin. |
| 10 | **Partly resolved** | The 9.1 and 9.2 checkpoints are fixed and the public payload example is planned. E15 still lacks an explicit kgwctl envelope and golden acceptance gate. Consumer review and final approval in the required order remain open. |
| 11 | **Partly resolved** | Two-module mutation, listener-name coverage, and the table fallback are covered. The plan tests `Guess` alone and leaves Snellius’s stats adapter and ambiguity handling untested. |

### New findings

- **P1 · Correctness · ECDS fallback is misstated.** Envoy can apply `default_config` after initial discovery fails even with `apply_default_config_without_warming` unset. The flag controls whether Envoy waits. The new rule can report an active fallback as unresolved. `docs/phases/phase-9/design.md:132`
- **P1 · Correctness · JSON projection erases selection diagnostics.** Pruning keeps selected routes, then projection removes references to excluded routes. That drops the ambiguous and unresolved results the new selection API exposes. Subject-only claim projection can also leave dangling `Evidence.Subject` IDs. `docs/phases/phase-9/design.md:259` · `docs/phases/phase-9/design.md:296`
- **P2 · Architecture · Filter links can cite unrelated policy records.** Compose checks the record’s module and the slot’s HCM, but never checks whether the record’s subject applies to the route. A record on an unrelated listener can pass and appear as a contribution. `docs/phases/phase-9/design.md:188`
- **P2 · Architecture · Two new API handoffs lack a dependency path.** The unchanged `Source` interface returns only a stream, leaving no declared handoff for capture completeness. `provenance.Notes` needs `render.Row`, while the package table permits provenance to import only model and chain. `docs/phases/phase-9/design.md:50` · `docs/phases/phase-9/design.md:60` · `docs/phases/phase-9/design.md:292`

The author reports `go test ./...` and `go vet ./...` passed. I reviewed the docs without rerunning tests or editing files.
## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| r1 | Listener blocks must follow shared route config sources | architecture | accept: §5 `Dependencies` item 2; plan 9.3 test 7 |
| r5 | Independently evidenced fields compete as one; evidence presence only | architecture | accept: `GatewaySource` split out; compose rule 3 checks named objects against evidence |
| r7 | Tests for source policy with zero and two uses | correctness | accept: plan 9.3 test 7 |
| r8 | `Unresolved` scope; ambiguity after `Known` | architecture | accept: scoped to query; `narrowed` label kept, certainty not raised |
| r9 | Plan omits ECDS, Composite, CONNECT, inline plugin cases | correctness | accept: plan 9.4 items 4, 5, 7 |
| r10 | kgwctl acceptance gate; review order | correctness | accept: plan "E15 acceptance". Review order stays an owner decision |
| r11 | Snellius stats adapter untested | correctness | reject: the adapter is Snellius code; xdsview tests `Guess` with cross-kind ambiguity (9.3 test 9) |
| N1 | P1 ECDS default applies without the flag | correctness | accept: §4 row 4 |
| N2 | P1 projection erases selection diagnostics; dangling evidence | correctness | accept: §7 projection rule; `selection` carries names; plan 9.3 test 11 |
| N3 | P2 links can cite unrelated records | architecture | accept: compose rule 4; plan 9.3 test 5 |
| N4 | P2 capture and `Notes` lack a dependency path | architecture | accept: `Source.Capture()`; provenance may import render |
