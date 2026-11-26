# Sub-phase 9.2 complete — pure execution

**Date:** 2026-11-25 | **Kind:** planned | **Base:** e019fdc | **Commits:** 5250313, 2cab37d, 3591396

## What changed
- `chain.Effective(snap) map[model.ID]model.EffectiveChain`, keyed by route ID. `chain.Annotate` is removed; `chain` imports only `model`.
- `ChainEntry` gains `Slot`, `Wrapper`, `Conditional`, `ConfigSource` (always `inline`); `Runs`, `Request`, `Response` are `model.Tri`. `model.Side` is gone. `Contributions` leaves the entry; the `Contribution` type stays in `model` until 9.3.
- A rejected per-filter config (`FilterConfig` with no config) gives `Runs: unknown`. `RequestOrder`, `ResponseOrder` and text keep it in position with `?` and its reason, never under `not run:`.
- Sides table names its Envoy version (`sidesEnvoyVersion`); `TestSides_CoverCorpus` checks every HTTP and upstream filter type in the corpus and fixtures. No exclusions needed.
- `render.Text(w, snap, chains, opts)`. `FilterRef`, `RouteRef`, `Extra`, `RouteExtra` are replaced by `Row` and the hooks `Style(Class, Row, string)`, `Notes(Row)`, `Labels(Row)`. `render` no longer imports `chain`.
- krp glue `cmd/krp/attribution.go`: `attribute` (today's logic from `chain/annotate.go`) and `contributions.labels`. `internal/renderer` takes a `View` (snapshot, chains, labels); the TUI navigates `Row`s.
- `internal/snaptest.IDs` gives hand-built test snapshots parser-style IDs.

## Tests
Failing before (local capture, not published):
```
boundary_test.go:53: chain depends on .../pkg/envoy/origin; pure core packages may not import provenance
boundary_test.go:53: render depends on .../pkg/envoy/origin; pure core packages may not import provenance
```
The other brief tests target an API that did not exist, so they failed to compile before.

Passing after: `TestPureCoreImportsNoProvenance`; `chain` `TestEffective_KeyedByRoute`, `_Wrappers`, `_Semantics` (Tri), `_Corpus03`/`_Corpus00` (slot IDs), `_SharedRouteConfig`, `TestSides_CoverCorpus`; `render` `TestText_RowsPerRoute`, `_UnknownRunState`, `_Hooks`; krp `TestAttribute_RowsPerRoute`, `TestText_Golden`. Attribution tests from `chain` (`annotate`, `certainty`, `corpus09`, `vendorneutral`, one in `review`) are ported to `cmd/krp/attribution_test.go`, none dropped. `go test ./...` and `go vet ./...` pass.

## Golden changes
- Text: none. The text golden test moved from `render` to `cmd/krp` (`golden_text_test.go`), since contribution labels are krp's.
- JSON (55 of 56; `.nomatch` has no routes): in every `effectiveChain` entry, `slot` and `configSource: "inline"` added, `runs` becomes `"yes"`/`"no"`; 06-multi-extproc's ext_proc entries gain `wrapper: "composite"`, `conditional: true`. Entry `contributions` moved to a top-level `contributions` object keyed by route ID, then slot ID. No other line changed (checked over the whole diff).

## Deviations
1. **Contributions moved, not kept in entries.** A krp-local type could only keep them inside entries by mirroring every snapshot type, which reorders keys. `schemaVersion` stays `v1alpha1` until 9.3's `v1alpha2`; nothing merges before then.
2. **`Labels` hook added beside `Notes`.** Design §7 makes `Notes` lines under a row. Today's contribution text sits on the row, so text parity needs an on-row hook. `Notes` covers route and filter rows; `Labels` covers every row.
3. **HCM default chain.** An HCM with no joined route config has its chain under its route config use ID, so render keeps printing it without computing chains itself.
4. **`Route.Chain` stays** in `model` as the JSON carrier; krp fills a copy at output. It goes with the other provenance fields in 9.3.

## For 9.3
- `provenance.Notes` must reproduce on-row text through `Labels`: route origins, vhost and listener origins still print from the `model` fields; move them to `Labels` when those fields go.
- krp's `attribute` and `contributionLabel` are the parity reference for `FilterLink` claims.
- Drop `Route.Chain` and the top-level `contributions` for `v1alpha2` `chains` and `claims`.
