# Phase 8 — Vendor-Neutral Core: implementation plan

Design: `design.md` (amended after the design review, D-014). There are three planned sub-phases, each a reviewable change of its own. Each starts with tests.
- A test for **changed** behaviour must fail before the change.
- A **replacement** test, which moves coverage off the enterprise corpus, must pass before the deletion.

Order (D-015): certainty first, because kgwctl `proxy routes` already ships unlabelled contributions (program edge E13). Then the corpus, then the mapper.

## 8.1 — Certainty and the filter link (R5, R7, R8, R9)

1. Tests first, through the composed path `origin.Annotate` then `chain.Annotate` (as `cmd/krp` runs it), at the JSON and text boundaries:
   1. **Filter link.**
      - One ext_authz instance with config: one policy contribution.
      - A route-level `global_disable/ext_auth` beside a vhost-level `ext_authz`, both matching `extAuth`: no contribution, and the policy is still listed. This fails today: it reverses `TestAnnotate_DisableBesideInheritedProvider` (6.5), and `TestAnnotate_Corpus09` loses the `public-no-extauth` link on `global_disable/ext_auth` (the policy stays under `policies:`).
      - Two instances at the same level: no contribution (unchanged).
   2. **Contribution certainty.**
      - The OSS mapper's contributions are `"certainty": "inferred"`.
      - A plugin whose `FilterOrigin` is `Exact` yields an `exact` extension contribution, so a hard-coded `inferred` fails.
   3. **`PolicyRef`.**
      - The built-in reader's refs are `exact`.
      - A metadata entry whose ID group/kind disagree with its `merge.` key is skipped. Fails today.
   4. **Route indices.**
      - `route_source` with a rule name and no `_rule-N` gives `indexCertainty: inferred`, and text `(from name)`.
      - Full `_rule-N`/`_match-N` gives no `indexCertainty`, and bare text.
      - Narrowing by rule index still selects the route in both cases.
   5. **Text.**
      - A single-reading route origin renders `… (inferred)`.
      - A `route_source` identity renders bare.
      - Ambiguous output is unchanged.
      - Listener and vhost origins render `(inferred)`.
      - A contribution renders `· TrafficPolicy ns/x (extAuth, inferred)`.
2. Implement design §3: the filter link, the three model fields, the reader check, and the doc comments.
3. Update `render`, and regenerate the text and JSON goldens with `-update`, enterprise goldens included (8.2 deletes them). Review the golden diff: it may hold only certainty fields, the suffixes, and contributions dropped by the one-candidate rule. Each dropped contribution is listed in the sub-phase record.
4. README: text output now shows certainty too.

**Done:**
- The tests above pass, and the golden diff matches step 3.
- Vet passes.
- `krp` text and TUI output is checked by hand on `09-kgwctl-policies`.

## 8.2 — Corpus out, OSS coverage kept (R1–R3)

1. Replacement tests, which pass on the 8.1 code:
   - a render check on the hand-built `accuracy` fixture: `router upstream filters (as configured): …`;
   - a neutral hand-built dump at `testdata/fixtures/upstream-extproc/envoy/config_dump.json`, with router upstream filters (typed config) and a route with `ExtProcPerRoute` `disabled`. Its JSON golden comes from `TestDumpJSON_Golden`. Check the golden for the `upstreamFilters` subtree and the per-route config.
2. Repoint the `parser_test.go` tests to OSS dumps (00 and 02_x on `kgateway-v2.5.0`). Enterprise-only expectations become the OSS values: the first filter on 443, and the rate-limit key `ratelimit/kgateway-system/ratelimit`.
3. Remove the enterprise globs and the skip from `parser/e2e_test.go`, `accuracy_e2e_test.go` and `filter/e2e_test.go`, and drop `TestFilter_E2E_EnterpriseScenarios`.
4. Delete `testdata/enterprise/` and `testdata/golden/enterprise/`. `TestDumpJSON_Golden` must still find every OSS dump.
5. Replace vendor names in the hand-built fixtures with neutral ones: `render_test.go`, `internal/renderer/renderer_test.go`, `resolve_test.go`, `tui_test.go`, `model/envoy_test.go`, `clone_test.go`, `origin_test.go`. Where a fixture's filter type matters to its output (sides), use a stock Envoy type, so expected output changes only by names. The `FilterTransformations` cases in `origin_test.go` stay until 8.3.
6. Docs: README corpus section, and a D-003 pointer to D-013.

**Done:**
- `grep -rniE 'enterprise|solo|gloo|_ee\b' --include='*.go' pkg cmd internal` finds only `origin/kgateway.go`'s transformation type and its tests, which 8.3 removes.
- The same grep over `testdata/` finds only recorded OSS manifests (the `solo-public/docs/hey` image). These are inputs, not library names.
- OSS goldens are unchanged.
- All tests and vet pass. Test counts per package are reported before and after, with each dropped test named and the reason given.

## 8.3 — Mapper vendor-neutral (R4, R6)

1. Tests first, through the composed path. Each is a hand-built route with `merge.*` metadata:
   - A `gateway.kgateway.dev/TrafficPolicy` `extAuth` field is attributed to the `ext_authz` entry (unchanged).
   - The same field from `example.com/TrafficPolicy` (wrong group) is in `Route.Policies` with no contribution. Fails today.
   - The same field from `gateway.kgateway.dev/AcmePolicy` (wrong kind) is in `Route.Policies` with no contribution. Fails today.
   - A `FilterTransformations` filter with a `transformation` field gets no contribution and shows unknown sides. Fails today.
   - The same, with a test plugin in `origin.Chain` whose `PolicyFilter` claims that type, is attributed. Passes once the plugin exists.
2. Restrict `PolicyFilter` to `gateway.kgateway.dev/TrafficPolicy`. Remove `transformationType` and its sides entry.
3. Direct `PolicyFilter` tests in `origin_test.go` pass full refs, and the `FilterTransformations` cases move to the plugin test.

**Done:** the tests above pass. OSS goldens are unchanged. `grep -rn FilterTransformations --include='*.go' .` finds only the plugin test. The 8.2 vendor grep finds nothing in `pkg cmd internal` outside that test. Vet passes.

## Close

1. Phase cross-model review of `f8b4af6..HEAD` (both dimensions), with triage.
2. `phase-8-complete.md`, roadmap, `phase-status.md`; close #32.
3. Tag `v0.6.0` (owner approval). `v0.5.0` is tagged after 8.1, following a cross-model review of `f8b4af6..HEAD` at that point (D-016).
4. Program handoff:
   - a `cross-repo` issue in kgwctl for each release: E13 with `v0.5.0`, E6 with `v0.6.0`, with its golden impact;
   - an E6/E13 row update sent through the program session, which owns `program.md` edits while it amends it.

## Order and size

8.1 → 8.2 → 8.3 (D-015).
- 8.1 regenerates every golden, enterprise ones included; 8.2 and 8.3 must leave the OSS goldens stable.
- 8.3 depends on 8.2's neutral fixtures.

Each sub-phase touches more than five files, mostly tests, and 8.1 adds three exported fields. Approving this plan approves that size and those API additions. No new dependencies.
