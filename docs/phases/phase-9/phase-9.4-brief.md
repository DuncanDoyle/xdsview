# Sub-phase 9.4 — Core correctness

Kind: planned
Base: b972693
Issues: none | Plan steps: implementation-plan.md §9.4 items 1–9; review 9.1–9.2 #5. Runs before 9.3 (D-021).

## Problem / goal
The core drops or misreads Envoy config it doesn't model: static listeners, non-HCM chains, ECDS filters, several matchers and route actions, default filter chains, cluster selectors. It can't say when an active HCM's route config is missing (E14), or which dump sections it ignored. Design §4 defines the behaviour of each row.

## Expected behaviour
Design §4 rows 1–9 are the expected behaviour, item by item. Also, from review 9.1–9.2 #5:

| Input / action | Expected |
|---|---|
| Scenario 00 `main_route` (static listener, inline config, also in `static_route_configs`) | One inline source `<HCM>/RCI` that records the static ID as `Reported`; the brief-9.1 check in `TestRouteConfigs_Scenario00` |
| Any non-HCM network filter | Has an occurrence and an ID (`L<n>/C<m>/N<k>`) |
| Missing RDS on a `CaptureComplete` dump, active listener | `RouteConfigState` `missing`, `rds-missing` warning |
| Same with unverified capture, absent `RoutesConfigDump`, a skipped entry, or a warming or draining listener | `unknown` |
| ECDS filter with no entry and no default | In position, `Runs` unknown, `State` unresolved, labelled in text, never shown as running |

## Root cause (if known)
`parser.go` reads only `dynamic_listeners` and HCM chains; matchers and actions are parsed partially; no section inventory.

## Acceptance tests
krp text and JSON goldens on scenario dumps and neutral hand-built fixtures (a static HTTP bootstrap, a tcp_proxy chain, an ECDS dump, matcher and action fixtures), plus parser API tests. Each fails before its change.

## Scope
In: `pkg/envoy/model`, `pkg/envoy/parser`, `pkg/envoy/chain` (ECDS run state), `pkg/envoy/render`, krp JSON output, new fixtures under `testdata/fixtures`.
Out: provenance, the kgateway module, `origin`/`filter` behaviour (9.3); krp `schemaVersion` stays `v1alpha1` until 9.3. Fixtures are neutral: no vendor or product names (P-010).

## Plan / design impact
D-021 (order). None otherwise.

## Definition of done
- [ ] each item's tests fail before, pass after (report both)
- [ ] full test suite and vet pass
- [ ] every changed golden listed with its reason in phase-9.4-complete.md
- [ ] phase-9.4-complete.md written; phase-status.md updated
