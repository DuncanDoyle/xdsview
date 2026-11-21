# Sub-phase 6.6 — Narrowing edge cases (`v0.4.2`)

Kind: stabilization
Base: 5d66c27
Issues: #29 | Source: kgwctl 3.2 Codex review

## Problem / goal
`filter.Ambiguous` can miss a route whose deleted reading is the requested HTTPRoute, once `Known` has reduced the candidates. Name matching on unannotated routes can match inside the vhost name.

## Expected behaviour

| # | Input | Expected |
|---|---|---|
| 1 | Route `…-httproute-a-b-c-d-0-0-matcher-0`, `Known` = `a/b-c-d`, `a-b/c-d`; request `a-b-c/d` | `Filter` selects nothing; `Ambiguous` lists the route |
| 1 | Same route, `Known` = `a/b-c-d` only (`NarrowedByKnown`); request `a-b-c/d` | `Filter` selects nothing; `Ambiguous` lists the route |
| 1 | Route with an `Exact` origin (route_source) for another HTTPRoute | `Ambiguous` doesn't list it |
| 1 | A route `Filter` selects | `Ambiguous` doesn't list it |
| 2 | Unannotated route `listener~80~httproute-orders-team-a-example-route-0-httproute-web-shop-0-0-matcher-0` in vhost `listener~80~httproute-orders-team-a-example`; request `team-a/orders` | Not selected; request `shop/web` selects it |
| 2 | Existing name-match cases (rule index, rule name) | Unchanged |

## Root cause (if known)
1. `Ambiguous` checks only the candidates left after `Known`.
2. `routeMatches` uses `strings.Contains` on the whole route name, which starts with the vhost name.

## Acceptance tests
`pkg/envoy/filter`, on annotated and unannotated snapshots.

## Scope
In: `pkg/envoy/filter`. Out: #30.

## Plan / design impact
None. `Ambiguous`'s doc names the readings it judges on.

## Definition of done
- [ ] acceptance tests fail before, pass after
- [ ] `go test ./...`, `go vet ./...` pass
- [ ] cross-model review of `5d66c27..HEAD`, triaged
- [ ] phase-6.6-complete.md; `v0.4.2` (owner OK given); #29 closed; kgwctl told
