Cross-model: YES (author: anthropic/claude-opus-5-5, reviewer: openai/gpt-6.2-sol, command: codex via run-reviewer.sh, status ok)
Scope: phase 8 fix round 4 (findings in phase-fixes3-codex.md); prompt limited to inputs kgateway v2.4/v2.5 or a public-API plugin can produce
Dimensions: correctness
Diff: 990b4e3..4df4bdf
Timing: on time
Local file links and vendor names redacted; reviewer text otherwise unchanged.

Five P1 correctness findings remain.

1. **P1 · `kgateway.go:193` · A valid v2.5 rule name can select the wrong rule.** For `HTTPRoute shop/web`, rule 1 can be named `httproute-web-shop-2-0-api`. v2.5 writes that full name in `route_source.rule` and writes `_match-0` separately. The mapper reads the name as v2.4 metadata and emits rule 2 named `api`. The conflict check then rejects the correct Envoy name. `--rule 2` selects rule 1, and `--rule httproute-web-shop-2-0-api` misses it. Mapper (`pkg/envoy/origin/kgateway.go:193`) · [v2.5 metadata writer](https://github.com/kgateway-dev/kgateway/blob/v2.5.1/pkg/kgateway/translator/irtranslator/policy.go#L147-L158)

2. **P1 · `kgateway.go:136` · A dotted v2.5 rule name loses numeric selection.** `api.v1` is a valid Gateway API rule name. With `route_source.rule: api.v1`, the mapper needs the Envoy name for the rule index. The DNS-label check discards the matching reading because it contains a dot. The exact route origin keeps index `-1`, so `--rule 1` omits the route even when its Envoy name identifies rule 1. Name parser (`pkg/envoy/origin/kgateway.go:136`) · [Gateway API `SectionName`](https://github.com/kubernetes-sigs/gateway-api/blob/v1.7.0/apis/v1/shared_types.go#L731-L759)

3. **P1 · `kgateway.go:165` · A valid core-group policy reference disappears.** A downstream policy plugin backed by a namespaced ConfigMap can emit `merge.ConfigMap` with ID `/ConfigMap/shop/settings`. That is the SDK’s encoding for an empty Kubernetes API group. The new `validGroupKind` check rejects it, removing an exact policy reference that `990b4e3` returned. Validator (`pkg/envoy/origin/kgateway.go:165`) · [Merge metadata writer](https://github.com/kgateway-dev/kgateway/blob/v2.5.1/pkg/kgateway/translator/irtranslator/policy.go#L106-L122) · [SDK policy ID](https://github.com/kgateway-dev/kgateway/blob/v2.5.1/pkg/pluginsdk/ir/gw.go#L42-L56)

4. **P1 · `kgateway.go:389` · An explicit module source bypasses the library check.** A v2.5 HCM plugin can supply `name: rust_module` together with `module.local.filename` pointing to a different shared library. Envoy gives `module` precedence over `name`. With `filter_name: rustformation`, the mapper still grants the OSS transformation link to that other library. Predicate (`pkg/envoy/origin/kgateway.go:389`) · [kgateway HCM plugin hook](https://github.com/kgateway-dev/kgateway/blob/v2.5.1/pkg/pluginsdk/ir/iface.go#L208-L211) · [Envoy module loader](https://github.com/envoyproxy/envoy/blob/v1.40.0/source/extensions/dynamic_modules/posix/dynamic_modules.cc#L356-L386)

5. **P1 · `kgateway.go:384` · A Composite filter running rustformation still loses its link.** A downstream plugin can use a Composite matcher whose `ExecuteFilterAction` delegates to the OSS `DynamicModuleFilter`. The parser exposes the delegate’s type in `f.Type` and retains the Composite config in `f.TypedConfig`. The module check only looks at the top level or a direct `extension_config.typed_config`, so a configured Composite with rustformation as its sole delegate loses the contribution it received at `36197a3`. Module check (`pkg/envoy/origin/kgateway.go:384`) · Parser delegation (`pkg/envoy/parser/parser.go:238`) · [Envoy `ExecuteFilterAction`](https://github.com/envoyproxy/envoy/blob/v1.40.0/api/envoy/extensions/filters/http/composite/v3/composite.proto#L95-L105)

Across the 51 recorded OSS dumps, all eight metadata-backed route origins retain their selection inputs, all 10 distinct policy IDs survive, and all five transformation instances still match. The 106 OSS goldens are byte-for-byte identical to `v0.5.0`.

By inspection, reverting the corresponding fixes makes the reported v2.4, invalid-group, hyphenated-kind, wrong-library, and direct matcher-wrapper cases fail. The two isolation cases catch removal of the exact-type and match-index checks independently. I relied on the author’s test and vet results and made no file edits.
## Triage
| # | Finding | Dimension | Source | Decision | Note |
|---|---|---|---|---|---|
| 1 | A v2.5 rule name shaped like v2.4 metadata is parsed as v2.4 | correctness | reviewer (P1) | accept | v2.4 writes no `match`; v2.5 always does. The v2.4 form is read only without `match`. `TestRoute_IndexCertainty`. Regression from 4df4bdf. |
| 2 | A dotted v2.5 rule name gets no index from the Envoy name | correctness | reviewer (P1) | defer: Phase 9 | Owner: "Defer the 3, close" (2026-11-23). |
| 3 | A core-group policy ID (empty group) is rejected | correctness | reviewer (P1) | accept | Empty group allowed when the key's group is empty. `TestPolicies_CertaintyAndKeyCheck`. Regression from 4df4bdf. |
| 4 | `module.local.filename` overrides the `rust_module` name | correctness | reviewer (P1) | defer: Phase 9 | Owner, as #2. |
| 5 | A Composite-wrapped rustformation loses its link | correctness | reviewer (P1) | defer: Phase 9 | Owner, as #2. |

#1 and #3 failed against 4df4bdf and pass now. No golden changed.
