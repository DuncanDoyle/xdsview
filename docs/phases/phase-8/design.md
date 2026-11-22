# Phase 8 — Vendor-Neutral Core: design

**Status:** draft for review (2026-11-22)
**Issue:** #32 | **Edge:** E6 (CLI move into kgateway) | **Decisions:** D-012, D-013
**Release:** `v0.5.0` (text output changes; JSON additive)

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
| R8 | No kgateway policy semantics beyond what the mapper already holds: no precedence, merge or inheritance logic is added. | program principle |

## Non-goals

- Rewriting git history to purge the old dumps. That is an owner decision, and not assumed.
- A sides hook for plugins (no consumer needs one).
- Certainty on `ObjectForName` (no consumer). It is tracked as a follow-up.
- A per-field basis for `RouteOrigin` indices parsed from the name when `route_source` lacks them (audit P2-c). This is documented, not modelled.

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
| `TestFilter_E2E_EnterpriseScenarios` | narrowing | `TestFilter_E2E_OSSScenarios` covers it; drop |
| `TestDumpJSON_Golden` (19 enterprise goldens) | JSON envelope | 106 OSS goldens |

### 2. Mapper (R4, R6, R8)

- `origin/kgateway.go` drops the envoy-gloo transformation type. `transformation` maps only to OSS kgateway's `dynamic_modules/simple_mutations` instance.
- `chain/sides.go` drops the `FilterTransformations` entry. That type then shows unknown sides (`?`), like any unlisted type.
- `PolicyFilter` applies `policyFilterKinds`, `globalDisable` and the transformation rule only when the policy is `gateway.kgateway.dev/TrafficPolicy`. `Policies` still reads every `merge.<Kind>.<group>` key, so another Kind is listed under `policies:` but gets no contribution unless a plugin's `PolicyFilter` claims it. The source for this: kgateway writes `merge.` metadata per policy GroupKind (`irtranslator/policy.go:19,106`, `v2.5.1`).
- A test plugin, composed with `origin.Chain`, attributes a non-OSS transformation type. This shows that R4's seam works without an exported API change.

### 3. Certainty (R5, R7)

- `model.Contribution` gains `Certainty model.Certainty` (`json:"certainty"`), with the same text values as origins.
  - Extension contribution: the `FilterOrigin`'s certainty. Today that is `inferred`, from the filter-name convention.
  - Policy contribution: `inferred`. The `merge.*` metadata proves that kgateway merged the field into the route, which keeps `Route.Policies` metadata-backed. The link to one filter entry comes from the mapper's table and the most-specific-level rule, so it is inference.
- Doc comments in `model/chain.go`, `chain/annotate.go` and `model/origin.go` say what each certainty rests on. A contribution "is attributed to", it doesn't "configure".
- `render`: an origin or contribution with certainty `inferred` gets the suffix ` (inferred)`. `ambiguous:` and `(chosen from known HTTPRoutes)` are unchanged. `exact` prints bare. This applies to listener, vhost and route origins, and to contributions. Listener and vhost origins are always `inferred`: the Gateway comes from the caller's options.
- `Route.Policies` and `PolicyRef` are unchanged. Their doc names the metadata they come from.

## Compatibility

| Consumer | Uses | Effect |
|---|---|---|
| kgwctl (`v0.4.3`) | `parser`, `origin.Kgateway`, `origin.Annotate`, `chain.Annotate`, `filter.*`, `render.Text` with `Style`/`RouteExtra`, `RouteAnnotation.Certainty`, `NarrowedByKnown`; `Contribution` in one test | Go API: additive. JSON: `certainty` added to each contribution. Text: ` (inferred)` added to origin and contribution lines, so goldens update. TrafficPolicy attribution unchanged (all 29 contribution kinds in its goldens are TrafficPolicy). |
| Snellius | none (no import today) | none. E4 can import `v0.5.0` directly. |
| enterprise dumps (anyone) | transformation attribution, sides | lost without a plugin (D-013) |

Exported Go API: no removals or signature changes. One field is added (`Contribution.Certainty`), so `Contribution` stays comparable.

## Data-plane cost

n/a: read-only tool.

## Risks

- A consumer that compares rendered text with string matching breaks on the suffix. kgwctl is the only one, and it is told before release.
- Tests repointed to OSS could lose a subtle enterprise-only parser path. The inventory found none beyond upstream filters and vendor names.
