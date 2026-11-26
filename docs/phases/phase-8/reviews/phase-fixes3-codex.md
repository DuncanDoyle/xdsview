Cross-model: YES (author: anthropic/claude-opus-5-5, reviewer: openai/gpt-6.2-sol, command: codex via run-reviewer.sh, status ok)
Scope: phase 8 fix round 3 (findings in phase-fixes2-codex.md)
Dimensions: correctness
Diff: 36197a3..990b4e3
Timing: on time
Local file links and vendor names redacted; reviewer text otherwise unchanged.

Four P1 correctness findings and two P3 test gaps remain.

1. **P1 — v2.4 rule metadata can still be overridden by a conflicting Envoy name.** kgateway v2.4 writes the rule name into `route_source.rule` when the rule is named. With metadata `httproute-web-shop-2-0-api` and an Envoy name ending in `httproute-web-shop-2-0-admin-matcher-0`, the mapper parses rule index `2` but loses both match `0` and rule name `api`. It then imports `admin`, so `--rule admin` selects the route. The unnamed legacy form `httproute-web-shop-2-0` also still accepts a name from Envoy. `pkg/envoy/origin/kgateway.go:187` · [kgateway v2.4 route-name writer](https://github.com/kgateway-dev/kgateway/blob/v2.4.7/pkg/kgateway/query/route.go#L90-L97)

2. **P1 — An invalid API group can still produce an exact policy reference.** `merge.AcmePolicy.bad_group.example` with ID `bad_group.example/AcmePolicy/ns/p` passes because the key and ID agree. Kubernetes rejects that API group, but the mapper emits an `Exact` reference for it. The kind regex also accepts overlength CRD kinds and drops valid hyphenated kinds such as `Acme-Policy`. `pkg/envoy/origin/kgateway.go:357` · [Kubernetes CRD validation](https://github.com/kubernetes/apiextensions-apiserver/blob/v0.34.2/pkg/apis/apiextensions/validation/validation.go#L350)

3. **P1 — A different shared library can still receive the OSS transformation link.** A configured `dynamic_modules/simple_mutations` filter with the exact Envoy type, `filter_name: rustformation`, and `dynamic_module_config.name: custom_module` passes the new predicate. Envoy loads the library named by `dynamic_module_config` and passes `filter_name` to that library. All five recorded OSS instances name `rust_module`. `pkg/envoy/origin/kgateway.go:326` · scenario 09 dump (`testdata/scenarios/09-kgwctl-policies/envoy/kgateway-v2.5.1/config_dump.json:4305`)

4. **P1 — A matcher-wrapped OSS transformation loses its link.** For an `ExtensionWithMatcher` wrapping the OSS `DynamicModuleFilter`, the parser exposes the delegate’s type in `f.Type` and retains the outer wrapper in `f.TypedConfig`. The new check looks for `filter_name` at the outer level, so a configured filter that linked at `36197a3` now loses its contribution even when the delegate specifies `rustformation`. `pkg/envoy/parser/parser.go:246` · `pkg/envoy/origin/kgateway.go:327`

5. **P3 — The lookalike-type test no longer isolates the exact-type check.** Its custom type has no `filter_name`, so restoring the earlier broad `filterKind` comparison leaves the test passing. Give the lookalike the same otherwise-valid configuration as the positive case. `pkg/envoy/chain/vendorneutral_test.go:99`

6. **P3 — The match-conflict test no longer isolates the match check.** It pairs unnamed metadata with a name containing `api`, so the new unnamed-rule guard rejects the reading even if the match-index comparison is removed. Metadata with rule `api` and match `_match-1`, paired with the existing `api` name for match `0`, would expose that regression through the inferred rule index. `pkg/envoy/origin/certainty_test.go:75`

I checked all 51 recorded OSS dumps against the changed predicates. The eight route-source records retain their selection inputs, all 10 distinct policy IDs survive, and all five transformation instances still match. The 106 OSS goldens are byte-for-byte identical to `v0.5.0`.

By inspection, the regression assertions for the reported v2.5 unnamed-rule case, the cross-kind fallback, the invalid namespace and name, and the different `filter_name` each fail against `36197a3`. The earlier vendor-source cleanup and upstream array golden coverage are intact. I relied on the author’s test and vet results and made no file edits.
## Triage
| # | Finding | Dimension | Source | Decision | Note |
|---|---|---|---|---|---|
| 1 | v2.4 `route_source.rule` with a rule name is misparsed; a conflicting name overrides it | correctness | reviewer (P1) | accept | Verified against kgateway v2.4.7 `query/route.go:90-97`: `<rule>-<match>[-<rule name>]`. The rule name is read; its absence marks the rule unnamed. Three v2.4 cases in `TestRoute_IndexCertainty`. |
| 2 | Invalid API group gives an exact ref; kind check wrong | correctness | reviewer (P1) | accept | Group must be a DNS subdomain; kind a DNS-1035 label once lowercased (CRD validation), so hyphenated kinds pass. `TestPolicies_CertaintyAndKeyCheck`. |
| 3 | Another library loading `rustformation` gets the link | correctness | reviewer (P1) | accept | The link also needs `dynamic_module_config.name: rust_module`, as in all recorded OSS instances. |
| 4 | Matcher-wrapped transformation lost its link (regression from 990b4e3) | correctness | reviewer (P1) | accept | The module check reads the wrapped filter's config. |
| 5 | Lookalike-type test didn't isolate the type check | correctness | reviewer (P3) | accept | Lookalikes carry otherwise valid config. #3–#5 are one table: `TestVendorNeutral_TransformationEvidence`. |
| 6 | Match-conflict test didn't isolate the match check | correctness | reviewer (P3) | accept | Metadata `api`/`_match-1` against a name for match 0. |

Two hand-built fixtures gained the `dynamic_module_config` that recorded dumps have. Every new case except the two isolation cases failed against 990b4e3. No golden changed.
