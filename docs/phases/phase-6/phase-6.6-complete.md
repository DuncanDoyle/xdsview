# Phase 6.6 — Narrowing edge cases (Complete)

**Completed:** 2026-11-21
**Range:** 5d66c27..v0.4.2

## Summary
`filter.Ambiguous` now reports a route that may belong to the requested HTTPRoute even after `Known` set that reading aside: Envoy can still serve a route for a deleted HTTPRoute. It judges on the mapper's candidates and on every reading of the route part of the name. Valid `route_source` metadata stays authoritative. Name matching on unannotated routes no longer matches inside the vhost name (#29, from kgwctl's 3.2 review).

## Sub-phases (phase record only)
n/a

## Reviews
- `reviews/6.6-codex.md`: Codex, cross-model. 4 findings (2 P1, 1 P2, 1 P3), all accepted and fixed.

## Waivers
none

## Deliverables

| Item | Test |
|---|---|
| `Known` keeping two other readings, or one, no longer hides the route | `TestAmbiguous_AllNameReadings` (failed before) |
| Selected routes and exact origins aren't listed | same test (guards) |
| Readings from the route part only; custom mapper candidates kept; `route_source` authoritative | `TestAmbiguous_ReviewCases` (all three failed before the review fix). Mutation: removing the `route_source` guard fails it. |
| Name match after `<vhost>-route-<n>-`; rule index and rule name unchanged | `TestFilter_NameMatchIgnoresVhost` (failed before), `TestFilter_NameMatchIgnoresVhostRules` |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

Changed expectation:
- **Test:** `TestFilter_AnnotatedOrigins`, the "known orders/team-a, request orders-team/a" case.
- **Change:** the route is now listed by `Ambiguous`.
- **Why:** that's #29's point, and the reviewer agreed.

## Notes for later phases
- Name readings use kgateway's naming convention. A route in another convention is judged on its mapper's candidates only.

## Deferred / open
- #30 (remaining policy fields) is open, waiting for a dump that uses them.
