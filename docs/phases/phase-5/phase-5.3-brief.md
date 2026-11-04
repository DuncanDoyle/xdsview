# Sub-phase 5.3 — `pkg/envoy/origin` and `--rule` by name in the filter

Kind: planned
Base: 08ec3e2
Issues: #16, program edge E5 | Plan steps: implementation-plan.md § 5.3

## Problem / goal
Consumers can't tell which Gateway API object produced an Envoy object. The UI's `EnvoyNamer` and kgwctl need a library that answers this, can be replaced, and accepts vendor plugins (P-001, P-002). Separately, the filter can't select an HTTPRoute rule by name.

## Expected behaviour

| Input / action | Expected |
|---|---|
| Route on any OSS recording, the scenario's HTTPRoutes known | That route's origin (HTTPRoute, rule, rule name, match), Inferred; Exact on `-route-source` recordings |
| Same, nothing known | The candidates contain that origin |
| Route with `route_source` metadata (either version's shape) | Exact; the name reading contains it |
| Policy filters (02_1, 02_7, 02_8, 03, 06) | Kind from the type, GatewayExtension from the name, `global_disable/*` a helper |
| Route metadata `merge.*` | Policy refs with their field |
| 05 virtual hosts, 07 clusters | Listener (port or section) plus hostname; Service plus port |
| Bare names: `kube_shop_x_8000`, `listener~8080`, `https`, `xds_cluster` | Service; Gateway; Gateway; false |
| `Chain(plugin, Kgateway)` through `Annotate` | Plugin origin on its filter, OSS origins elsewhere; input unchanged |
| `filter.Options{RuleName: "a"}` on 08 | Only rule `a`'s route, on both versions |

## Root cause (if known)
n/a (new capability)

## Acceptance tests
`pkg/envoy/origin/e2e_test.go` (literal per-scenario tables over every OSS recording), `origin_test.go` (seam, `ObjectForName`, a rule name containing digits), `TestFilter_E2E_RuleName`, and the boundary test including `origin`.

## Scope
In: `pkg/envoy/model` (origin types and fields), `pkg/envoy/origin`, `pkg/envoy/filter`, `pkg/envoy/boundary_test.go`.
Out: the CLI (5.4).

## Plan / design impact
One deviation: `Certainty` marshals to JSON as a name ("inferred"), not a number. The `Mapper` methods return one struct with a `Certainty` field rather than a `(value, Certainty)` pair, so a struct can be stored on the model directly.

## Definition of done
- [ ] acceptance tests pass; mutations show they bite
- [ ] full test suite passes
- [ ] phase-5.3-complete.md written; phase-status.md updated
