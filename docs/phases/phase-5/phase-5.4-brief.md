# Sub-phase 5.4 — CLI: origins and `--rule <name>`

Kind: planned
Base: 331461a
Issues: #16 | Plan steps: implementation-plan.md § 5.4

(Written at the start of 5.4, committed with the work.)

## Problem / goal
`krp dump` doesn't show which Gateway API objects produced what it prints, and `--rule` takes only an index.

## Expected behaviour

| Input / action | Expected |
|---|---|
| `--route X --route-ns N` on a policy dump | Route line shows `HTTPRoute N/X · rule R · match M`; policy filters show `← GatewayExtension ns/name`; a `Policies:` block lists `TrafficPolicy ns/name (field)` |
| A `route_source` dump, no flags | Route origin lines without `--route` |
| No `--route`, no metadata | No route origin line (ambiguous); filter and policy lines still shown |
| `--rule a-b` on 08 | One route, labelled `rule 1 (a-b)` |
| `--rule 0`, `--rule -1`, `--rule -2` | Index 0; all rules; error |
| `-o json` | Origins and policies added, nothing else changed |
| A plugged-in mapper | Its origin reaches the JSON output |

## Root cause (if known)
n/a

## Acceptance tests
`TestDumpText_Origins`, `TestDumpJSON_PluginOrigin`, `TestRender_Origins`, `TestRender_AmbiguousOriginHidden`, and the golden files.

## Scope
In: `cmd/krp`, `internal/renderer`, `testdata/golden`, README, roadmap.
Out: discovering HTTPRoutes from the cluster (see the phase-status note).

## Plan / design impact
None.

## Definition of done
- [ ] acceptance tests pass
- [ ] full test suite passes
- [ ] phase-5.4-complete.md written; phase-status.md updated
