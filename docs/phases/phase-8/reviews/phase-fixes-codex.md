Cross-model: YES (author: anthropic/claude-opus-5-5, reviewer: openai/gpt-6.2-sol, command: codex via run-reviewer.sh, status ok)
Scope: phase 8 fix round (findings in phase-codex.md)
Dimensions: correctness
Diff: 0a1ae02..431f363
Timing: on time
Local file links and vendor names redacted; reviewer text otherwise unchanged.

**Hold `v0.6.0` for the remaining attribution bugs.** No recorded OSS route loses a selection. I compared the old and new selection inputs across all 51 recorded OSS dumps. The eight route records with exact `route_source` metadata produce identical candidates, and routes without exact metadata follow the same selection path as before.

### Findings

1. **P1 · correctness · `kgateway.go:234` — An unnamed rule can still supply the index for a named rule.** With `route_source` naming rule `api` and match `_match-0`, a route name ending in `httproute-web-shop-2-0-matcher-0` parses as unnamed rule 2. The `c.RuleName != ""` check lets that reading through. The mapper emits `rule 2 (api)`, and `--rule 2` selects it. The new regression test covers a different nonempty rule name, so it misses this variant of finding #1. `pkg/envoy/origin/kgateway.go:234` · `pkg/envoy/filter/narrow_test.go:256`

2. **P1 · correctness · `kgateway.go:349` — A policy ID with no kind still becomes an exact reference.** Under `merge..example.com`, the ID `example.com//shop/p` passes because the key and ID both have an empty kind. The reader emits an `Exact` `PolicyRef` even though the record cannot identify a Kubernetes resource. Finding #2’s tests catch the original empty namespace and empty name cases. The reader also needs to reject an empty kind. `pkg/envoy/origin/kgateway.go:349` · `pkg/envoy/origin/certainty_test.go:13`

3. **P1 · correctness · `kgateway.go:321` — A custom protobuf type can still receive the built-in OSS transformation link.** A configured filter named `dynamic_modules/simple_mutations` with type `type.googleapis.com/example.filters.http.dynamic_modules.v1.OtherFilter` receives a contribution because `filterKind` extracts `dynamic_modules` from any matching package. Finding #3 requires evidence that the filter is Envoy’s `DynamicModuleFilter`. Both new negative cases lack the `.filters.http.dynamic_modules.` segment, so they pass with the overly broad check. `pkg/envoy/origin/kgateway.go:279` · `pkg/envoy/origin/kgateway.go:321` · `pkg/envoy/chain/vendorneutral_test.go:99`

4. **P2 · correctness · `implementation-plan.md:69` — The 8.3 exit gate still expects the vendor literal that finding #4 removed.** The public Go source passes the D-019 hygiene check, and the synthetic test still exercises plugin attribution. The plan still expects one hit for the downstream transformation type name, so the implemented fix and the written exit gate disagree. The close record acknowledges the change. `docs/phases/phase-8/implementation-plan.md:69` · `docs/phases/phase-8/phase-8.3-complete.md:20`

### Regression and test checks

- `possibleOrigins` now returns only mapper candidates for an exact origin, but its sole caller, `Ambiguous`, already skips exact origins before calling it. Removing that guard would leave the new narrowing test passing. The test exercises `Filter`, which rejects the original conflicting rule through the metadata mapper. `pkg/envoy/filter/filter.go:153` · `pkg/envoy/filter/filter.go:257`
- By inspection, the new route-conflict, incomplete-ID, and same-name/different-type assertions all fail against `0a1ae02`. The identifier cleanup is verified by a source scan. Finding #5 is resolved at the CLI JSON boundary. Dropping or flattening the two-element array under `upstreamFilters[].typedConfig` would fail the golden comparison. Finding #6 is recorded for P-009 as triaged. `cmd/krp/main_test.go:112` · `testdata/golden/fixtures/upstream-extproc.json:88` · `docs/phase-status.md:39`

I used the author’s test and vet results and made no file edits.
## Triage
| # | Finding | Dimension | Source | Decision | Note |
|---|---|---|---|---|---|
| 1 | A name reading an unnamed rule can supply the index for a named rule | correctness | reviewer (P1) | accept | Verified. With a metadata rule name, any name reading with a different or no rule name conflicts. Cases added to `TestRoute_IndexCertainty` and `TestFilter_NameConflictsWithMetadata`. |
| 2 | A policy ID with an empty kind becomes an exact ref | correctness | reviewer (P1) | accept | Verified. An empty kind is skipped too. Case added to `TestPolicies_CertaintyAndKeyCheck`. |
| 3 | Any `*.filters.http.dynamic_modules.*` type gets the transformation link | correctness | reviewer (P1) | accept | Verified. The link needs Envoy's exact `DynamicModuleFilter` type. Case added to `TestVendorNeutral_TransformationNeedsType`. |
| 4 | The 8.3 plan gate still expects the vendor literal | correctness | reviewer (P2) | accept | Plan gate amended. |
| 5 | The `possibleOrigins` exact guard is unreachable (its caller skips exact origins) | correctness | reviewer (note) | accept | Guard removed; `filter.go` is back to its 0a1ae02 state. The origin fix alone resolves phase finding #1. |

Each new case fails against 431f363 and passes after the fix. No golden changed.
