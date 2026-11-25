# Sub-phase 9.3 — Provenance contract and kgateway module

Kind: planned
Base: 029ade1
Issues: none | Plan steps: implementation-plan.md §9.3 tests 1–12; review 9.1–9.2 #3 (sources keep route-config metadata), #4 (render hook for the policies line); consumer reviews (reviews/consumer-*.md)

## Problem / goal
Provenance still lives in `origin` and `filter`, writes into the model (`Route.Origin`, `Policies`, `Chain`, `Listener.Origin`, …), composes first-wins, and krp glue computes contributions. Design §5–§7 define the replacement: typed claims with evidence, explicit composition, an index with dependencies and gaps, a kgateway module, consumer-side selection, a `NameMapper`, and krp JSON `v1alpha2`.

## Expected behaviour
Design §5 (Claim, Evidence, Module, Compose rules 1–4, assertion kinds and cardinality, Index, Resolve, Dependencies with `Dependent` and `Gap`, expected gaps incl. `policyRecord`, `dropped`, `gatewaySource`), §6 (module, `merge.*` at all levels, filter-link rule, Phase 8 P1s #2/#4/#5, Select/Narrow rules 1–6, `Names`, `Guess`), §7 (render `Row`, `Notes`, `Labels`, kgwctl additions 1–8, krp flow, projection rule, `v1alpha2`) are the expected behaviour. Plan §9.3 tests 1–12 are the acceptance tests.

| Parity case | Expected |
|---|---|
| Every scenario through krp's new path | Text golden unchanged, except where a Phase 8 P1 fix applies or the policies line moves for a stated reason |
| krp `--route`, `--rule`, live `Known` | Same routes as today; ambiguous, unresolved and narrowed listed in `selection` |
| Static bootstrap listeners | No origin claims; dropped under `--route`, as today |

## Root cause (if known)
`origin.Chain` first-wins (`origin.go`), provenance fields in `model`, krp `attribution.go` glue.

## Acceptance tests
Plan §9.3 tests 1–12, at the public API of `provenance` and `adapter/kgateway` (contract tests in external test packages) and krp text and JSON goldens.

## Scope
In: new `pkg/envoy/provenance`, new `pkg/adapter/kgateway`; `model` (remove provenance fields, add lookups, `SourcePath`, `CompareID`, warning-code constants, route-config source metadata); `render` hooks; `cmd/krp`, `internal/renderer`, `internal/tui`; delete `pkg/envoy/origin`, `pkg/envoy/filter`, `cmd/krp/attribution.go`; README and JSON schema doc.
Out: new parser behaviour beyond what claims need; core-correctness changes (done in 9.4).

## Plan / design impact
none

## Definition of done
- [ ] plan §9.3 tests 1–12 fail before (or don't compile), pass after
- [ ] `go test ./...`, `go vet ./...` pass; `go list -deps` of model, parser, chain, render shows no provenance, adapter or third-party package
- [ ] every changed golden listed with its reason in phase-9.3-complete.md
- [ ] phase-9.3-complete.md written; phase-status.md updated
