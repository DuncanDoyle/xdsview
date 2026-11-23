# Sub-phase 8.3 — Mapper vendor-neutral

Kind: planned
Base: 251a451
Issues: #32 (part) | Plan steps: implementation-plan.md §8.3 | Edge: E6 | Decisions: D-012, D-013, D-018

## Problem / goal
The core kgateway mapper links any policy Kind to a filter, and it knows an enterprise transformation filter type (P-002). Policy links come only from kgateway's own TrafficPolicy. The enterprise type leaves the core; a plugin in `origin.Chain` can add it back.

## Expected behaviour
Each case is a hand-built route with `merge.*` metadata, run through `origin.Annotate` then `chain.Annotate`.

| # | Input / action | Expected |
|---|---|---|
| 1 | `gateway.kgateway.dev/TrafficPolicy` with `extAuth`, one configured `ext_authz` entry | Policy contribution on that entry (inferred) |
| 2 | The same field from `example.com/TrafficPolicy` (wrong group) | In `Route.Policies` (exact); no contribution |
| 3 | The same field from `gateway.kgateway.dev/AcmePolicy` (wrong kind) | In `Route.Policies` (exact); no contribution |
| 4 | A `FilterTransformations` filter, TrafficPolicy `transformation` field | No contribution; request and response sides unknown |
| 5 | Case 4 with a test plugin in `origin.Chain` whose `PolicyFilter` claims that type | Policy contribution on that entry (inferred) |
| 6 | OSS goldens | Unchanged |

## Root cause (if known)
n/a (planned change).

## Acceptance tests
Cases 2–4 fail before the change. Tests run through the composed path, as krp does.

## Scope
In: `origin/kgateway.go`, `chain/sides.go`, their tests.
Out: Policies reading (any Kind stays listed), render, parser.

## Plan / design impact
None.

## Definition of done
- [ ] cases 1–5 pass; OSS goldens unchanged
- [ ] `grep -rn FilterTransformations` over Go code finds only the plugin test
- [ ] the 8.2 vendor grep over `pkg cmd internal` finds nothing outside that test
- [ ] `go test ./...`, `go vet ./...` pass
- [ ] phase-8.3-complete.md; phase-status updated
