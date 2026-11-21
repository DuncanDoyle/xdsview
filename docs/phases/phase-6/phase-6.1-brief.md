# Sub-phase 6.1 — Model and parser

Kind: planned
Base: 5d82113
Issues: #27 (E11) | Plan steps: implementation-plan.md § 6.1

## Problem / goal
The parser reads only `active_state`. A listener that is warming, draining or rejected comes out as an empty listener with no state, and an LDS-wide error is lost. The parser also reads `typed_per_filter_config` only on routes. 6.2 needs the virtual host, route config and weighted cluster levels to resolve the effective chain. This sub-phase adds the model types from the design and fills listener state and the three new per-filter config levels from the dump.

## Expected behaviour

| Input (`dynamic_listeners` entry or RDS) | Expected in the parsed snapshot |
|---|---|
| `active_state` only | `State.Status: active`, chains from `active_state`, `LastUpdated` from it |
| `warming_state` only | `Status: warming`, chains from `warming_state` |
| `draining_state` only | `Status: draining`, chains from `draining_state` |
| `active_state` + `warming_state` | `Status: active`, `Updating: true`, chains from `active_state` |
| `active_state` + `draining_state` | `Status: active`, `Draining: true`, chains from `active_state` |
| `error_state` only | `Status: rejected`, no chains, `Error{Details, LastAttempt}` |
| `active_state` + `error_state` | `Status: active`, chains from `active_state`, `Error` set |
| an entry with no `name` and an `error_state` (LDS-wide error) | a `rejected` listener with an empty `Name` and `Error` set |
| `typed_per_filter_config` on a virtual host, a route config, a weighted cluster | `TypedPerFilterConfig` on `VirtualHost`, `RouteConfig`, `WeightedCluster`, present on the HCM's joined `RouteConfig` (after the per-HCM clone) |
| a route config shared by two HCMs, one copy mutated | the other HCM's copy of the new maps is unchanged |
| corpus dumps (`testdata/scenarios`) | `krp dump -o json` goldens change by additions only: `state` on every listener |

The model also gains `EffectiveChain`, `ChainEntry`, `Contribution`, their enum types, `RequestOrder` and `ResponseOrder` (design § Model). `Route.Chain` stays nil until 6.2's `chain.Annotate`. `RequestOrder` keeps entries with `Runs` and `Request != no` in HCM order. `ResponseOrder` keeps entries with `Runs` and `Response != no`, reversed.

## Root cause (if known)
n/a (new behaviour). `rawListener` declares only `active_state` (`pkg/envoy/parser/parser.go:366`).

## Acceptance tests
- `pkg/envoy/parser`: table test over synthetic `config_dump` documents, one per listener row above, through `parser.Parse`.
- `pkg/envoy/parser`: per-filter config at the three levels, asserted on `Listeners[].FilterChains[].HCM.RouteConfig` through `parser.Parse`. The clone test covers mutation isolation.
- `pkg/envoy/model`: `RequestOrder` and `ResponseOrder` on a literal chain with `unknown`, `no`, and non-running entries.
- JSON goldens in `testdata/golden/` regenerated; the diff checked with jq for additions only.

## Scope
In: `pkg/envoy/model`, `pkg/envoy/parser`, JSON goldens.
Out: `chain`, `render`, `filter` narrowing, `krp` text output (6.2 and 6.3).

## Plan / design impact
None.

## Definition of done
- [ ] acceptance tests fail before, pass after (report both)
- [ ] `go test ./...` and `go vet ./...` pass
- [ ] cross-model review at the phase review (planned sub-phase)
- [ ] phase-6.1-complete.md written; phase-status.md updated
