# Phase 5.3 — `pkg/envoy/origin` and `--rule` by name in the filter (Complete)

**Completed:** 2026-11-05
**Range:** 8bbcf9a..accf3a7 (branch `duncandoyle/phase5`, pending design approval)

## Summary
`pkg/envoy/origin` answers "which Gateway API object produced this Envoy object?" for listeners, route configs, virtual hosts, routes, clusters, HTTP filters and policies. Every answer states how certain it is.

- `Kgateway` is the OSS mapper.
- `Chain` composes it with vendor plugins.
- `Annotate` writes the answers into a copy of a snapshot.
- `ObjectForName` serves the UI's `EnvoyNamer`, which works on bare names.

`filter.Options.RuleName` selects a rule by name (#16).

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Route origins on every OSS recording | `TestKgateway_RouteOrigins`: 50 runs (24 scenarios × 2 versions, plus the two `-route-source` recordings). Every route must be in the literal table, and every table entry must be in the dump. |
| Metadata vs name | `TestKgateway_ExactMatchesInferred` on both `-route-source` recordings |
| Filters, policies, vhosts, clusters | `TestKgateway_PolicyFilters`, `TestKgateway_Policies`, `TestKgateway_VirtualHostsAndClusters` |
| Plugin seam, `ObjectForName`, rule name with digits | `origin_test.go` (synthetic dump, independent of `testdata/enterprise`) |
| Rule by name | `TestFilter_E2E_RuleName` on 08, both versions |
| Tests bite | Four mutations, each caught and reverted: <ul><li>ignore v2.4's `rule` format</li><li>drop the `Known` filtering (shows the 4 readings of `api-example-com-kgateway-system`)</li><li>leave `_` in hostnames</li><li>prefix-only rule-name match (keeps `a-matcher-b` for `a`)</li></ul> |
| Boundary | `origin` in `TestCoreImportsOnlyStdlib` |
| tests / vet | `go test -count=1 ./...` ok; `go vet ./...` clean |

## Notes for later phases
- **Ambiguity is the common case.** Nearly every real route name has several readings: `api-example-com` in `kgateway-system` reads 4 ways. Without `Known` HTTPRoutes or `route_source` metadata, route origins are Ambiguous. That drives the CLI question in phase-status Notes.
- **A finding while writing the tests.** `ObjectForName("xds_cluster")` first returned the Gateway, because any bare name was read as a section name. Section names must now be DNS labels.
- **Deviation from the design** (in the 5.3 brief): `Certainty` marshals as a name, and `Mapper` methods return one struct with a `Certainty` field.

## Deferred / open
- #16 to be closed when this branch reaches `main` and the CLI flag lands (5.4).
