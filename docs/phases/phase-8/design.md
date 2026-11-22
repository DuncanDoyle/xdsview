# Phase 8 — Vendor-Neutral Core: design

**Status:** approved (2026-11-22, D-016)
**Issue:** #32 | **Edge:** E6 (CLI move into kgateway) | **Decisions:** D-012, D-013, D-014 (design-review amendments), D-015 (sub-phase order)
**Release:** `v0.5.0` after 8.1 (certainty; text output changes, JSON additive), `v0.6.0` after 8.3 (D-016)

## Goal

The module ships no vendor or edition data or names in its core (program P-002). Its attribution never states more than its evidence: the kgateway controller owns policy semantics, and xdsview reports what Envoy runs plus labelled links back to the resources.

## Requirements

| # | Requirement | Source |
|---|---|---|
| R1 | No enterprise dumps, manifests or goldens in the module. | #32, D-013, E6 |
| R2 | OSS regression coverage stays: every behaviour the enterprise tests protect is tested on OSS dumps or hand-built fixtures, at the same boundary. | #32 |
| R3 | No vendor or edition names in non-test code or in hand-built fixtures. | P-002 |
| R4 | The enterprise transformation type is recognised only through a plugin, and the `origin.Chain` seam stays able to do so. | #32 |
| R5 | Every attribution carries a certainty in the model and JSON. A policy contribution is at most `inferred`. | D-012, audit P1-a, P2-a |
| R6 | The OSS policy-field table applies only to kgateway's own policy Kind. | D-012, audit P1-b |
| R7 | Text output marks `inferred` origins and contributions, as it marks `ambiguous`. | D-012, audit P2-b, P2-d |
| R8 | No kgateway policy semantics beyond what the mapper already holds. No precedence, merge or inheritance logic is added, and the level rule that assumed one is removed. | program principle, D-014 |
| R9 | A policy is linked to a filter only when the dump leaves one candidate. Each `PolicyRef` states its certainty, and so do route indices that came from the name. | D-014 |

## Non-goals

- Rewriting git history to purge the old dumps. That is an owner decision, and not assumed.
- A sides hook for plugins (no consumer needs one).
- Certainty on `ObjectForName` (no consumer). It is tracked as a follow-up.

## Design

### 1. Corpus (R1–R3)

- Delete `testdata/enterprise/` and `testdata/golden/enterprise/`.
- Tests move to OSS dumps per the inventory below. `TestParseAllScenarios` and `TestDumpJSON_Golden` walk `testdata/scenarios` only.
- Hand-built fixtures that name a vendor policy or filter (`EnterpriseKgatewayTrafficPolicy`, `io.solo.*`, `solo.io`, `_ee`) switch to a neutral example (`AcmePolicy`, `example.com`).

| Enterprise-backed test | Behaviour | OSS replacement |
|---|---|---|
| `TestParseAllScenarios`, matcher 01_1–01_9 | parse every dump, matchers | already on OSS 2.4.7 and 2.5.0; drop the enterprise glob |
| `TestParse_SimpleHTTP`, `_HTTPSWithSNI`, `_ExtAuth`, header modifiers, mirror, rewrite, CORS, rate limit | parser accuracy | OSS 00, 02_1–02_8. Enterprise-only expectations (first filter `io.solo.transformation`, key `ratelimit_ee/default`) become the OSS values |
| `TestE2E_UpstreamFilters` | router upstream filters parsed **and rendered** (#15) | No OSS dump has them. The hand-built `accuracy` fixture already covers parsing; add a render check on it |
| `TestDumpJSON_Golden` on `06-multi-extproc` | CLI JSON for `upstreamFilters` (typed config, origin) and per-route `ExtProcPerRoute.disabled` | a neutral hand-built dump under `testdata/fixtures/`, with its own JSON golden, added before the deletion |
| `TestFilter_E2E_EnterpriseScenarios` | narrowing | `TestFilter_E2E_OSSScenarios` covers it; drop |
| `TestDumpJSON_Golden` (19 enterprise goldens) | JSON envelope | 106 OSS goldens |

### 2. Mapper (R4, R6)

- `origin/kgateway.go` drops the envoy-gloo transformation type. `transformation` maps only to OSS kgateway's `dynamic_modules/simple_mutations` instance.
- `chain/sides.go` drops the `FilterTransformations` entry. That type then shows unknown sides (`?`), like any unlisted type.
- `PolicyFilter` applies `policyFilterKinds`, `globalDisable` and the transformation rule only when the policy is `gateway.kgateway.dev/TrafficPolicy`. `Policies` still reads every `merge.<Kind>.<group>` key, so another Kind is listed under `policies:` but gets no contribution unless a plugin's `PolicyFilter` claims it. The source for this: kgateway writes `merge.` metadata per policy GroupKind (`irtranslator/policy.go:19,106`, `v2.5.1`).
- A test plugin, composed with `origin.Chain`, attributes a non-OSS transformation type. This shows that R4's seam works without an exported API change.

### 3. Certainty and the filter link (R5, R7, R8, R9)

- **Filter link.** `chain` links a policy field to a filter entry only when exactly one entry on the route matches it (`PolicyFilter`) and has per-filter config for the route, at whatever config level. An entry on the HCM default is not a candidate. Two or more candidates, at any levels: no contribution. The policy stays on `Route.Policies`. The most-specific-level rule goes (D-014).
- **`model.Contribution`** gains `Certainty model.Certainty` (`json:"certainty"`), with the same text values as origins.
  - Extension contribution: the `FilterOrigin`'s certainty. The OSS mapper's is `inferred`, from the filter-name convention. A plugin's may be `exact`.
  - Policy contribution: `inferred`. The metadata records the field against the route. The link to the one filter comes from the mapper's field table.
- **`model.PolicyRef`** gains `Certainty` (`json:"certainty"`). The built-in reader returns `exact`, and only for an ID whose group and kind match its `merge.<Kind>.<group>` key; it skips the others. Its claim is that the dump records this policy ID against this field on the route, no more. A plugin sets its own certainty.
- **`model.RouteOrigin`** gains `IndexCertainty model.Certainty` (`json:"indexCertainty,omitempty"`). It is `inferred` when a rule index, match index or rule name came from the route name under a `route_source` identity, and unset otherwise (the annotation's certainty applies). `Certainty` stays `Exact` for the identity, so metadata-first narrowing is unchanged.
- **Doc comments** in `model/chain.go`, `model/origin.go`, `chain/annotate.go` and `origin/kgateway.go` say what each certainty rests on. A contribution "is attributed to" a filter; it doesn't "configure" it.
- **`render`.** An origin, contribution or policy with certainty `inferred` gets ` (inferred)`; `exact` prints bare; `ambiguous:` and `(chosen from known HTTPRoutes)` are unchanged. Route indices with `IndexCertainty` `inferred` print `· rule 1 · match 0 (partly from name)`: the qualifier covers the group, meaning at least one of the rule index, rule name or match index came from the name. A policy or contribution that is neither exact nor inferred prints its certainty (`unknown`, `ambiguous`). A policy link is no stronger than its `PolicyRef`, and a filter configured on a weighted backend counts as a candidate (8.1 review #3, #4). This covers listener, vhost and route origins, contributions, and `policies:`. Listener and vhost origins are always `inferred`, because the Gateway comes from the caller's options.

## Compatibility

| Consumer | Uses | Effect |
|---|---|---|
| kgwctl (`v0.4.3`) | `parser`, `origin.Kgateway`, `origin.Annotate`, `chain.Annotate`, `filter.*`, `render.Text` with `Style`/`RouteExtra`, `RouteAnnotation.Certainty`, `NarrowedByKnown`; `Contribution` in one test | Go API: additive. JSON: `certainty` on each contribution and policy, and `indexCertainty` on route origins built partly from the name. Text: ` (inferred)` on origin and contribution lines, and `(partly from name)` on such indices, so goldens update. All 29 contribution kinds in its goldens are TrafficPolicy, so the Kind restriction drops none. A contribution where several filters match the field is dropped (D-014); dump 09 keeps its links (D-017). |
| Snellius | none (no import today) | none. E4 can import `v0.5.0` directly. |
| enterprise dumps (anyone) | transformation attribution, sides | lost without a plugin (D-013) |

Exported Go API: no removals or signature changes. Fields are added to `Contribution`, `PolicyRef` and `RouteOrigin`. All three stay comparable (`Certainty` is an integer type).

## Data-plane cost

n/a: read-only tool.

## Risks

- A consumer that compares rendered text with string matching breaks on the suffix. kgwctl is the only one, and it is told before release.
- Tests repointed to OSS could lose a subtle enterprise-only parser path. The inventory found none beyond upstream filters and vendor names.
