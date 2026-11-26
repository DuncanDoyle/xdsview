# Phase re-review 6 — Phase 9 (Codex)

**Scope:** c73e4d3..77215bd
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-26

Three P1 findings and one P2.

- **P1 · correctness. Static listeners can hide dynamic route inputs.** A bootstrap listener can subscribe to RDS. The parser puts the downloaded routes under the static listener’s ID, and `bootstrap` exempts every descendant by ID prefix. With no route-config-level per-filter entries, `Dependencies` can report zero gaps for an unattributed xDS route and its backends. [index.go:325](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/provenance/index.go:325) · [parser.go:591](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/parser/parser.go:591)
- **P1 · architecture. `Complete` can certify a candidate set the producer says is incomplete.** A single valid `Exact` claim with `Incomplete: true` yields `Complete: true` and `covered`. An inferred claim also gets `Complete: true` by default, without the module affirming that it enumerated every split. A consumer checking all candidates can therefore authorize a partial set. [index.go:643](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/provenance/index.go:643) · [index.go:669](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/provenance/index.go:669)
- **P1 · correctness. The override label confuses filter enablement with policy execution.** The label uses the level that decides whether the slot runs. Envoy’s CORS filter can still take `allow_methods` from a less-specific config when the more-specific config leaves it empty. Header mutation applies every level’s mutations. Both can label a policy `overridden` while it still affects requests. [text.go:118](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/provenance/text.go:118) · [text.go:281](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/provenance/text.go:281) · [Envoy CORS](https://github.com/envoyproxy/envoy/blob/v1.39.6/source/extensions/filters/http/cors/cors_filter.cc#L235) · [Envoy header mutation](https://github.com/envoyproxy/envoy/blob/v1.39.6/source/extensions/filters/http/header_mutation/header_mutation.cc#L184)
- **P2 · correctness. A dropped mirror claim is reported against the primary backend.** Drop handling omits `Gap.Cluster`, so a rejected mirror claim can mark a covered primary backend `dropped` while the mirror itself reports `noEvidence`. The block stays withheld, but the per-input reason points to the wrong backend. [index.go:584](/Users/ddoyle/Development/claude/kgateway-filterchain-printer-cli-claude/pkg/envoy/provenance/index.go:584)

`Inputs` and `Dependencies` share one enumeration. The kind capture confirms scoped policy records and the `headerModifiers` link. The sides entry matches Envoy’s factory registration, and I found no kgateway dependency in core execution. Static review only. Tests and vet passed per the author. No files edited.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| 1 | Static listeners can hide dynamic route inputs | correctness | accept: fixed in ae0955d |
| 2 | `Complete` can certify a candidate set the producer says is incomplete | architecture | accept: fixed in aefc86c |
| 3 | The override label confuses filter enablement with policy execution | correctness | accept: fixed in 330d271 |
| 4 | A dropped mirror claim is reported against the primary backend | correctness | accept: fixed in fdf708f |