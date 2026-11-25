# Sub-phase 9.1 — Snapshot and occurrence IDs

Kind: planned
Base: 8d381f6
Issues: none (tracked in phase-status.md) | Plan steps: implementation-plan.md §9.1 tests 1–9

## Problem / goal
Results and attribution are keyed by names today. Duplicate RDS names silently overwrite each other (`parser.go:100`), and a shared route config is cloned per HCM with no record that the uses share one source. Nothing tells a complete dump from a filtered one. Every later Phase 9 piece keys on occurrence IDs, so they come first (design §2).

## Expected behaviour

| Input / action | Expected |
|---|---|
| Parse any scenario dump | Every listener, filter chain, network filter, route config use, vhost, route, weighted backend, HTTP and upstream filter slot has an ID, unique in the snapshot |
| `ID.Within` | A route is within its vhost, HCM, chain and listener; not within a sibling; `L1` does not contain `L10/...` |
| Two HCMs on one RDS name | One source in `Snapshot.RouteConfigs`; two uses with distinct vhost and route IDs, each `Source` pointing at it |
| Two dynamic sources with one name | Both listed; the referencing HCM's use is `ambiguous`, not joined; a warning with the use as subject |
| Scenario 00 `main_route`, inline and in `static_route_configs` | One inline source (`<HCM>/RCI`) that records the static ID as `Reported` |
| A static entry with no inline match | A source with no use; an RDS name never resolves to it |
| Existing warnings | Same text, plus a code and, where it has one, a subject ID |
| `model.Prune` | Keeps selected routes and ancestors, every non-active listener, and active listeners warming, draining or with an error; IDs unchanged |
| Parse one dump twice | Deep-equal, IDs included |
| `source.File` / `Admin` / `kube` | `Capture()` returns unverified / complete / complete; `parser.Options.Capture` defaults to unverified |

## Root cause (if known)
Name-keyed RDS index (`parser.go:100`) and per-HCM clone (`parser.go:177`); no occurrence identity in `model`.

## Acceptance tests
Public API of `model`, `parser`, `source`, plus krp JSON goldens. The duplicate-name and source/use tests fail before the change.

## Scope
In: `pkg/envoy/model`, `pkg/envoy/parser`, `pkg/envoy/source` (Capture), mechanical adaptation of `origin`, `filter`, `chain`, `render`, `cmd/krp`, `internal/*` to the renamed snapshot.
Out: `chain.Effective` changes, render hooks (9.2); provenance and the module (9.3); core-correctness items (9.4) — static listener *parsing*, ECDS, matchers. `filter` keeps its own pruning in 9.1; `model.Prune` is added and tested beside it.

## Plan / design impact
none

## Definition of done
- [x] acceptance tests fail before, pass after (report both)
- [x] full test suite and vet pass
- [x] krp text goldens unchanged except duplicate-RDS cases (listed); JSON goldens gain IDs, `routeConfigs`, capture, typed warnings
- [x] phase-9.1-complete.md written; phase-status.md updated
