# Phase 8 — Vendor-Neutral Core: implementation plan

Design: `design.md`. Three planned sub-phases, each a reviewable change of its own. The tests come first in each one. A test that guards a change must fail before the change.

## 8.1 — Corpus out, OSS coverage kept (R1–R3)

1. Port `TestE2E_UpstreamFilters`'s render assertion onto the hand-built `accuracy` fixture. The render boundary must print `router upstream filters (as configured): …`. It passes on the current code; the parser side is already covered by `TestParse_UpstreamFiltersAndTypes`.
2. Repoint the `parser_test.go` tests to OSS dumps (00, 02_x on `kgateway-v2.5.0`). Change the enterprise-only expectations to the OSS values: the first filter on 443, and the rate-limit key `ratelimit/kgateway-system/ratelimit`.
3. Remove the enterprise globs and the skip from `parser/e2e_test.go`, `accuracy_e2e_test.go` and `filter/e2e_test.go`, and drop `TestFilter_E2E_EnterpriseScenarios`.
4. Delete `testdata/enterprise/` and `testdata/golden/enterprise/`. `TestDumpJSON_Golden` must still find every OSS dump.
5. Rename vendor names in the hand-built fixtures: `render_test.go`, `internal/renderer/renderer_test.go`, `resolve_test.go`, `tui_test.go`, `model/envoy_test.go`, `clone_test.go`, `origin_test.go`. Expected outputs change only by the names.
6. Docs: README corpus section, and a D-003 pointer to D-013.

**Done:** `grep -rniE 'enterprise|solo|gloo|_ee\b|io\.solo' --include='*.go' --include='*.json' --include='*.yaml' .` finds nothing outside `docs/`. All tests and vet pass. Test count per package is reported, with each dropped test named and the reason given.

## 8.2 — Mapper vendor-neutral (R4, R6, R8)

1. Tests first, at the `chain.Annotate` boundary on a hand-built route with `merge.*` metadata:
   - A `TrafficPolicy` `extAuth` field is attributed to the `ext_authz` entry (unchanged).
   - An `AcmePolicy` (another group/Kind) with an `extAuth` field gets no contribution, but is still listed in `Route.Policies` (fails today).
   - A filter of type `FilterTransformations` with a `transformation` field gets no contribution and shows unknown sides (fails today).
   - The same, with a test plugin in `origin.Chain` whose `PolicyFilter` claims that type, is attributed (passes once the plugin exists).
2. Restrict `PolicyFilter` to `gateway.kgateway.dev/TrafficPolicy`. Remove `transformationType` and its sides entry.
3. Move the `FilterTransformations` cases in `origin_test.go` to the plugin test.

**Done:** the tests above pass, all OSS goldens are unchanged, `grep FilterTransformations pkg/ cmd/ internal/ --include='*.go'` finds only the plugin test, and vet passes.

## 8.3 — Certainty on attribution (R5, R7)

1. Tests first:
   - JSON: each contribution in a `chain.Annotate` result carries `"certainty": "inferred"`, both on a hand-built route and in the OSS goldens (`02_7`, `09-kgwctl-policies`).
   - Text: a route origin from a single name reading renders `… (inferred)`; a `route_source` origin renders bare; an ambiguous origin is unchanged.
   - Text: a listener or vhost origin renders `(inferred)`; a contribution renders `· TrafficPolicy ns/x (extAuth, inferred)`.
2. Add `Contribution.Certainty`, set it in `chain/annotate.go`, and fix the doc comments (design §3).
3. Update `render` and regenerate the text and JSON goldens with `-update`. Review the golden diff: only the suffixes and the `certainty` fields change.
4. README: certainty now shows in text output too.

**Done:** the tests above pass. The golden diff holds only certainty changes. Vet passes. `krp` text and TUI output is checked by hand on `09-kgwctl-policies`.

## Close

1. Phase cross-model review of `<base>..HEAD` (both dimensions), with triage.
2. `phase-8-complete.md`, roadmap, `phase-status.md`; close #32.
3. Tag `v0.5.0` (owner approval).
4. Program handoff:
   - a `cross-repo` issue in kgwctl naming E6 and `v0.5.0`, with its golden impact;
   - an E6 row update sent through the program session, which owns `program.md` edits while it amends it.

## Order and size

8.1 → 8.2 → 8.3. 8.2 depends on 8.1's neutral fixtures, and 8.3 changes the goldens that 8.1 and 8.2 must leave stable. Each sub-phase touches more than five files, mostly tests. Approving this plan is the approval for that size. No new dependencies.
