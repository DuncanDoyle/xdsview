# Phase 5.4 — CLI: origins and `--rule <name>` (Complete)

**Completed:** 2026-11-05
**Range:** 331461a..96ec2a2 (branch `duncandoyle/phase5`, pending design approval)

## Summary
`krp dump` labels routes with their HTTPRoute rule and match, policy filters with their `GatewayExtension`, and routes with their policies. `-o json` carries every origin with its certainty. `--rule` accepts a rule name as well as an index.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Origins with `--route` | `TestDumpText_Origins/known_route` (02_7 v2.5.0) |
| `route_source` dump, no flags | `TestDumpText_Origins/route_source_metadata` (04 v2.4.7) |
| Ambiguous hidden | `TestRender_AmbiguousOriginHidden` |
| `--rule a-b` | `TestDumpText_Origins/rule_by_name`: one route, `rule 1 (a-b)` |
| `--rule 0 / -1 / -2` | By hand on 04: 1 route, 4 routes, error. `-2` is in `TestDumpOutput_Rejected`. |
| JSON additions only | 71 golden files changed. With `origin` and `policies` removed, each equals its old version (jq, 0 differences). The line diff's 238 "deletions" are commas. |
| Plugin reaches output | `TestDumpJSON_PluginOrigin` (golden `02_7…/kgateway-v2.5.0.plugin.json`) |
| tests / vet | `go test -count=1 ./...` ok; `go vet ./...` clean |

## Notes for later phases
- `-o json` without `--route` lists every reading of each route name, four for `api-example-com` in `kgateway-system`, so the JSON grows. That's by design (§ Ambiguity), and it's what a consumer with its own known HTTPRoutes needs.
- The 5.4 brief was written at the start of the work and committed with it, not in a separate setup commit.

## Deferred / open
- #16 to be closed when this branch reaches `main`.
