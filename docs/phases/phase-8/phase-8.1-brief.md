# Sub-phase 8.1 — Certainty on attribution, one-candidate filter link (`v0.5.0`)

Kind: planned
Base: d39f5db
Issues: #32 (part) | Plan steps: implementation-plan.md §8.1 | Edge: E13 | Decisions: D-012, D-014, D-015, D-016

## Problem / goal
kgwctl `proxy routes` shows policy-to-filter links and route indices as fact, although some are inferred from a field table or parsed from a route name. Consumers can't tell evidence from inference. When several filters match a policy field, xdsview picks one using an assumption about kgateway's translation.

## Expected behaviour

| # | Input (composed path: `origin.Annotate` then `chain.Annotate`) | Expected |
|---|---|---|
| 1 | Route with `merge.TrafficPolicy.gateway.kgateway.dev` `extAuth` → `ns/x`; one `ext_authz` entry configured for the route | One contribution `TrafficPolicy ns/x (extAuth)`, JSON `"certainty": "inferred"`; text `· TrafficPolicy ns/x (extAuth, inferred)` |
| 2 | Same field; route-level `global_disable/ext_auth` and vhost- or route-config-level `ext_authz` both match | No contribution on either entry; `Route.Policies` still lists `ns/x` |
| 3 | Same field; two matching instances at one level | No contribution (unchanged) |
| 4 | Plugin in `origin.Chain` whose `FilterOrigin` is `Exact` | Its extension contribution has `"certainty": "exact"`, text bare |
| 5 | OSS mapper extension contribution (filter-name convention) | `"certainty": "inferred"` |
| 6 | `merge.` entry whose ID group/kind match the key | `PolicyRef` with `"certainty": "exact"`; text under `policies:` bare |
| 7 | `merge.` entry whose ID group or kind disagrees with the key | Entry skipped; not in `Route.Policies` |
| 8 | `route_source` metadata with a rule name and no `_rule-N` in the Envoy name | `RouteOrigin.Certainty` exact, `"indexCertainty": "inferred"`; text `· rule 1 · match 0 (from name)` |
| 9 | `route_source` with full `_rule-N`/`_match-N`, and nothing taken from the name | No `indexCertainty` in JSON; text bare. A rule name taken from the name still makes it `inferred` (design §3; 8.1 review #5) |
| 10 | Narrowing by HTTPRoute rule index on cases 8 and 9 | Route selected in both (unchanged) |
| 11 | Route origin from the name only, single reading | Text `… (inferred)` |
| 12 | Ambiguous route origin | Text unchanged (`ambiguous: …`) |
| 13 | Listener and vhost origins | Text `(inferred)` |
| 14 | `09-kgwctl-policies` dump | `global_disable/ext_auth` row keeps `TrafficPolicy kgwctl-proxy/public-no-extauth (extAuth, inferred)`: the provider has no per-filter config on that route, so the helper is the one candidate |

## Root cause (if known)
- `model.Contribution`, `PolicyRef` and `RouteOrigin` carry no certainty (audit P1-a, P2-a, design-review #1, #3).
- `chain.attribute` breaks a multi-candidate tie by config level (`chain/annotate.go`, 6.5).
- `origin` `Policies` accepts any `merge.` key without checking the ID against it.
- `render` prints `Exact` and `Inferred` origins the same way.

## Acceptance tests
- `chain` and `origin` tests through the composed path, asserting `Route.Chain` contributions and `Route.Policies`.
- `cmd/krp` JSON and text goldens (`TestDumpJSON_Golden` and the text goldens), regenerated and reviewed.
- `render` tests for the text forms.
- `filter` narrowing tests for case 10.

Cases 1, 2, 4–9, 11, 13 and 14 fail before the change. Cases 3, 10 and 12 are guards.

## Scope
In: `pkg/envoy/model`, `pkg/envoy/chain`, `pkg/envoy/origin`, `pkg/envoy/render`, `pkg/envoy/filter` (tests only), `cmd/krp` goldens, `internal/` where it renders these, README.
Out: the enterprise corpus (8.2), the TrafficPolicy-only mapper rule and the transformation type (8.3), `ObjectForName` certainty, #26, #30.

## Plan / design impact
None beyond D-014 to D-016. Golden diff may hold only certainty fields, the suffixes, and contributions dropped by the one-candidate rule; each dropped contribution is listed in the complete record.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] `go test ./...`, `go vet ./...` pass
- [ ] `krp` text and TUI checked by hand on `09-kgwctl-policies`
- [ ] cross-model review of `f8b4af6..HEAD` (both dimensions), triaged, before `v0.5.0` (D-016)
- [ ] phase-8.1-complete.md; `v0.5.0` (owner OK given in D-016); kgwctl `cross-repo` issue for E13 (ask before filing)
