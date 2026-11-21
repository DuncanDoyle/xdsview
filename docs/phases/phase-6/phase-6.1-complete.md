# Phase 6.1 — Model and parser (Complete)

**Completed:** 2026-11-21
**Range:** 5d82113..45e1f43 (`main`)

## Summary
The parser now shows a listener's active version, else its warming one, else its draining one, and records that as `Listener.State`. An active listener with a newer version warming is `Updating`, and one with an older version draining is `Draining`. A listener with only an `error_state` is `rejected` with no chains, and so is an LDS-wide error without a name. `typed_per_filter_config` is read on route configs, virtual hosts and weighted-cluster backends, and each HCM gets its own copy. The model has the `EffectiveChain` types and `RequestOrder`/`ResponseOrder`. `Route.Chain` stays nil until 6.2.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Listener state, 8 dump shapes | `TestParse_ListenerState` (`pkg/envoy/parser/state_test.go`). Before the parser change all 8 cases failed: empty `State`, and no chains or address for warming and draining. After the change all pass. Shapes checked against Envoy `listener_manager_impl.cc` at `c7c57d8f48` (L555-599). |
| Per-filter config at three levels, on the joined route config | `TestParse_PerFilterConfigLevels`. It failed before the change (empty maps) and passes after. |
| Shared route config: each HCM owns its maps | `TestParse_PerFilterConfigIsolated`. Before the change it panicked on a nil map; it passes after. The existing `clone_test.go` tests are unchanged and pass. |
| `RequestOrder` / `ResponseOrder` | `TestChainOrders` (`pkg/envoy/model/chain_test.go`): drops non-running entries and entries without that side, keeps `unknown`, and reverses the response order. |
| JSON goldens additive | 72 goldens regenerated. With `state` and `effectiveChain` stripped, each equals HEAD under `jq -S`. All 74 listeners are `active`. No corpus dump has per-filter config at vhost, route config or backend level, which matches the design's corpus claim. |
| tests / vet | `go test ./...` all ok; `go vet ./...` clean |

Other changes:
1. **`decodeAll`** replaces the two copies of the decode loop for raw maps. It returns nil when nothing decodes, where the old loop returned `{}`. JSON output is the same because of `omitempty`.
2. **`cloneRouteConfig`** uses `maps.Clone`.
3. **Two parser tests** compare `[]WeightedCluster` with `reflect.DeepEqual`, because the type holds a map and is no longer comparable.

## Notes for later phases
- `WeightedCluster` isn't comparable any more. External code that compared it with `==` or `slices.Equal` breaks at compile time. Consumers weren't checked for this; mention it in the `v0.4.0` notice.
- `ListenerState.Status` has no `omitempty`, so every listener in `-o json` carries `state`.

## Deferred / open
none
