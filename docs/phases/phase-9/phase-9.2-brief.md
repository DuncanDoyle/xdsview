# Sub-phase 9.2 — Pure execution

Kind: planned
Base: d944818
Issues: none | Plan steps: implementation-plan.md §9.2 tests 1–5

## Problem / goal
`chain` imports `origin`: `chain.Annotate` calls the mapper and writes contributions into execution entries (P-010 rule 1). Render hooks key on index tuples, so a slot printed under two routes can't be told apart. Run state is boolean, so the core can't say "unknown" (rule 3). Design §3 and §7.

## Expected behaviour

| Input / action | Expected |
|---|---|
| `boundary_test.go` | Fails if `model`, `parser`, `chain` or `render` import `origin`, `filter`, `provenance` or `adapter/...`. Fails today (`chain` imports `origin`) |
| `chain.Effective(snap)` | One chain per route ID; each entry carries its slot ID; `Runs`, `Request`, `Response` are `Tri`; values match today's for every scenario |
| Composite filter | `Wrapper: composite`, `Conditional` |
| ExtensionWithMatcher | `Wrapper: extensionWithMatcher`, `Conditional`; sides are its inner filter's |
| `render.Text(w, snap, chains, opts)` | Hooks `Style(Class, Row, string)` and `Notes(Row)`; with krp's notes, every scenario's text golden is unchanged |
| One slot under two routes with different per-filter config | Each row gets its own notes |
| Sides table check | Every HTTP filter type in the corpus and fixtures is in the table or a recorded exclusion; the table names its Envoy version |

## Root cause (if known)
`chain/annotate.go` (mapper call, contributions on entries); `render.FilterRef`/`RouteRef` index tuples; `ChainEntry.Runs bool`.

## Acceptance tests
Boundary test, `chain` public API, krp text and JSON goldens. The boundary test fails before the change.

## Scope
In: `pkg/envoy/model` (chain types), `pkg/envoy/chain`, `pkg/envoy/render`, `pkg/envoy/boundary_test.go`, `cmd/krp` glue that computes contributions from `origin` and feeds `Notes`, `internal/renderer`, `internal/tui`. Attribution tests move onto that krp glue.
Out: provenance package and kgateway module (9.3); ECDS `ConfigSource` and unresolved state (9.4: field exists, stays empty/inline); `origin` and `filter` stay until 9.3.

## Plan / design impact
none

## Definition of done
- [ ] boundary test fails before, passes after (report both)
- [ ] full test suite and vet pass
- [ ] text goldens unchanged; JSON goldens change only in chain shape (slot IDs, `Tri`, `wrapper`, `conditional`, no contributions inside entries; contributions appear wherever krp's JSON now carries them — list it)
- [ ] phase-9.2-complete.md written; phase-status.md updated
