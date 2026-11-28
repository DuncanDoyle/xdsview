# Review: kgwctl notes (Notes label, merged index certainty)

Reviewer: Codex (`codex exec --sandbox read-only`), cross-model: yes. Author: Claude Opus 5.5.
Scope: `91f00af..HEAD` on `duncandoyle/kgwctlNotes`, after phase 9, from two kgwctl notes (2026-11-27). Four rounds.

## Round 1 (base `91f00af`)

The merge fix works for the kgateway module’s documented outputs, including three-way merges in any order. It can discard stronger index evidence from claims that `Compose` accepts. The label constants are additive, but changing the value returned by `Notes` breaks callers that match the documented v0.7.0 label.

- **P1 · correctness · Unset can override stronger index evidence.** Two agreeing `Inferred` claims with unset and `Exact` index certainty resolve to unset, losing the explicit `Exact`. `Compose` checks only the outer certainty, so it also admits `Ambiguous` index certainty, and an existing selection test supplies `Exact` indices on an `Inferred` claim. Compare effective certainties before choosing the result, or explicitly define and enforce the narrower field contract. [index.go:144](pkg/envoy/provenance/index.go) · [compose.go:260](pkg/envoy/provenance/compose.go) · [select_test.go:117](pkg/adapter/kgateway/select_test.go)
- **P1 · correctness · Narrowed text bypasses the merge.** If `Known` drops route B and keeps two claims for the same reading of route A that differ only in `Exhaustive`, `Labels` compares the entire struct and still prints `ambiguous: HTTPRoute …`. D-022’s krp selection counts those claims as one reading. This pre-existing path needs the same merge rule. [text.go:218](pkg/envoy/provenance/text.go) · [output.go:122](cmd/krp/output.go)
- **P2 · architecture · The rename changes the public `Notes` contract.** The constants give Go callers a shared spelling. kgwctl’s old match stops firing, though its displayed output stays the same because its replacement already equals the new label. Other relabelers can silently lose their override. Keep the existing hook behavior in v0.7.1 and expose the new wording additively, or record the program decision P-010 requires for another break. [text.go:43](pkg/envoy/provenance/text.go) · [kgwctl text.go:109](kgwctl: internal/proxyview/text.go) · [P-010 rule 7](https://github.com/DuncanDoyle/kgateway-tooling/blob/06d814eb0219c6e6fc0e55374c580d7ff149051c/program.md#L200)
- **P2 · correctness · The scope wording is still inconsistent.** `Records` says the policies “apply to a route,” and `Notes` still calls the accumulator `inherited`. The Phase 9 design still names the old label. The README and text golden are updated, and CLI JSON emits raw claims with policy levels. [index.go:774](pkg/envoy/provenance/index.go) · [text.go:60](pkg/envoy/provenance/text.go) · [design.md:561](docs/phases/phase-9/design.md)

`Resolve` merges only claims tied at the highest outer certainty. Within that group, unset is absorbing, `max` handles explicit values, and both operations are associative and commutative. Claim order still determines how distinct candidates are listed. `git diff --check` passed. I used the program guidelines from the local checkout’s `origin/main` and relied on the author’s test results.
## Round 2 (base `74e04c5`)

- **P1 · correctness · Mixed-certainty `Narrowing.Kept` makes labels order-dependent.** For two agreeing readings, an `Exact` claim with explicit `Inferred` indices and an `Ambiguous` claim with unset indices, `(partly from name)` appears only when the `Ambiguous` claim comes last. The merge uses the incoming claim’s certainty for both readings. `kgateway.Narrow` supplies only top-certainty claims, but the public `Narrowing` type has no such restriction. [text.go:227](pkg/envoy/provenance/text.go) · [index.go:812](pkg/envoy/provenance/index.go) · [select.go:97](pkg/adapter/kgateway/select.go)
- **P1 · correctness · `Resolve` can erase explicit index evidence.** Two agreeing `Inferred` claims with explicit `Inferred` index certainty previously retained that value. The fix rewrites it to unset, which removes `(partly from name)` when rendered. [index.go:150](pkg/envoy/provenance/index.go) · [text.go:254](pkg/envoy/provenance/text.go)
- **P2 · correctness · The wording fix is incomplete.** The `Records` comment and `Notes` accumulator are corrected. The Phase 9 design still specifies `inherited policies:` for `Notes`. [design.md:561](docs/phases/phase-9/design.md)

I traced both tests against `74e04c5`. `TestResolve_IndexCertainty` gets `Unknown` instead of `Exact` in both input orders. `TestText_NarrowedAgreeing` gets an ambiguous label because the old whole-struct comparison treats the flag differences as distinct readings. `git diff --check` passed. I relied on the author’s test runs and made no file edits. The `Notes` compatibility decision remains with the owner.
## Round 3 (base `6f8fca3`)

- **P1 · Correctness · Narrowed labels can falsely attribute indices to a name.** Two agreeing `Kept` claims with unset `IndexCertainty`, one `Inferred` from a module table and one `Ambiguous`, merge to explicit `Inferred`. The renderer then adds `(partly from name)` even though neither claim used name evidence. [index.go:145](pkg/envoy/provenance/index.go) · [text.go:256](pkg/envoy/provenance/text.go)
## Round 4 (base `c7a6c4b`)

- **P2 · correctness.** The Phase 9 design still specifies keeping the highest `IndexCertainty` field. HEAD compares effective certainty and can keep the field unset. For example, an `Exact` claim with unset indices beats an agreeing claim with explicit `Inferred` indices and keeps the field unset. The documented rule would preserve the name attribution that this fix removes. [design.md:304](docs/phases/phase-9/design.md)
## Triage

| Round | # | P | Dimension | Finding | Decision |
|---|---|---|---|---|---|
| 1 | 1 | P1 | correctness | unset beats an explicit `Exact` index | accept: compare effective certainty (`6f8fca3`) |
| 1 | 2 | P1 | correctness | narrowed label compared whole structs, not readings | accept: merge as in `Resolve` (`6f8fca3`) |
| 1 | 3 | P2 | architecture | new label value changes `Notes` output | owner: kgwctl already prints the same wording; release choice with the owner |
| 1 | 4 | P2 | correctness | "inherited" left in `Records` comment, accumulator name | accept (`6f8fca3`) |
| 2 | 1 | P1 | correctness | mixed-certainty narrowed merge depends on order | accept: per-claim certainty (`c7a6c4b`) |
| 2 | 2 | P1 | correctness | explicit `Inferred` on an inferred claim erased | accept (`c7a6c4b`) |
| 2 | 3 | P2 | correctness | design still names the old label | accept (`c7a6c4b`) |
| 3 | 1 | P1 | correctness | two unset readings merge into explicit `Inferred` ("partly from name") | accept: keep the winner's own value (`1821423`) |
| 4 | 1 | P2 | correctness | design still says "highest `IndexCertainty`" | accept: design amended |
