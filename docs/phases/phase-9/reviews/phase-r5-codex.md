# Phase re-review 5 — Phase 9 (Codex)

**Scope:** c19e12e..2fcea2c
**Cross-model:** yes. cli: codex; resolved_model: gpt-6.2-sol; status: ok
**Date:** 2026-11-25

Local file links and vendor names redacted; reviewer text otherwise unchanged.

**R6 still has P1 blockers.** The enumeration misses inputs visible in text and JSON, and the `Reported` fix can omit evidence from an inline source’s listener.

| Round 4 triage item | Status |
|---|---|
| r8 · Scoped `merge.*` kind capture | **Not resolved.** Still deferred to the owner. Completion record `docs/phases/phase-9/phase-9.3-complete.md:152` |
| P2 · Reported static copy treated as unused | **Partly resolved.** The copy now uses inline placement evidence. It still misses the inline source’s listener lineage. `pkg/envoy/provenance/index.go:388` |
| P3 · Quadratic listener route-list rebuild | **Resolved.** The index builds route lists once and checks only the selected route for a route block. `pkg/envoy/provenance/index.go:373` |
| P1 · Upstream slot evidence outside route scope | **Resolved.** Upstream slots now contribute their claim objects and get a gap when attribution is missing. `pkg/envoy/provenance/index.go:427` |
| P1 · Weighted overrides and slot-less entries skipped | **Resolved.** Both now produce policy gaps. `pkg/envoy/provenance/index.go:403` · Weighted overrides `pkg/envoy/provenance/index.go:443` |

### New findings

- **P1 · Correctness.** A `Reported` static block can now clear its policy gaps using readable inline placement evidence while omitting an unreadable Gateway or listener policy. The alias lookup adds the inline source and serving slots, but skips the ancestor evidence that §5 requires the two source blocks to share. `pkg/envoy/provenance/index.go:388` · Design §5 `docs/phases/phase-9/design.md:224`
- **P2 · Correctness.** The corpus table’s “Before” counts for `hostSource` and `extensionSource` should be **0 → 204** and **0 → 51**. The base had no host-gap rule and required per-filter overrides for an extension gap. Bootstrap gaps have no Kubernetes origin, and captured Gateway context cannot close a static listener’s gap. The empty-route placement gap also predates this fix. Corpus gaps `docs/phases/phase-9/phase-9.3-complete.md:158`
- **P3 · Correctness.** The property test gets its removal candidates from `ix.inputs` itself and checks only route blocks. An omitted input class generates zero cases. The assertion also accepts any new gap, so it can pass when the affected input’s own gap is absent. `pkg/envoy/provenance/export_test.go:6` · `pkg/envoy/provenance/failclosed_test.go:30`

### Remaining R6 blocker

**P1 · Architecture and correctness.** Both renderers expose mirror destinations, but the enumeration checks only primary and weighted clusters. With the other inputs covered, Snellius can show a route whose mirror Service the viewer cannot read. Two corpus dumps contain `httpbin-mirror`, so the corpus write-up’s “No (a) cause” misses a concrete coverage failure. Non-HCM network filters are also visible in both outputs and lack a provenance input. `pkg/envoy/provenance/index.go:368` · Text renderer `pkg/envoy/render/render.go:531` · Mirror JSON `testdata/golden/scenarios/02_4-single-policy/kgateway-v2.5.0.json:61` · Network-filter fixture `testdata/golden/fixtures/tcp-proxy.txt:2`

Static review. The author reports passing tests and vet. The diff changes no goldens, and I edited no files.

## Triage

| # | Finding | Dimension | Decision |
|---|---|---|---|
| r8 | Kind capture for scoped `merge.*` still open | correctness | defer: kind capture to the owner |
| P1 | `Reported` static block omits the inline source's listener lineage | correctness | accept: fixed in f71f195 |
| P2 | Corpus table's before-counts and static-listener note wrong | correctness | accept: fixed in 3bcb71a |
| P3 | Property test draws candidates from `ix.inputs`, checks route blocks only, accepts any new gap | correctness | accept: fixed in 0b983f1 (JSON oracle for completeness in f71f195) |
| P1 | Mirror clusters and non-HCM network filters are no input | architecture, correctness | accept: fixed in f71f195 (design §5 in 210eff4) |
