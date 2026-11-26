# Sub-phase 8.2 complete — downstream corpus out, OSS coverage kept

**Date:** 2026-11-22 | **Kind:** planned | **Base:** da82d0d | **Edge:** E6 (part) | **Decisions:** D-013, D-018

## Outcome
The former downstream corpus directory (19 dumps) and its goldens (19) are gone from the tree. Git history keeps them, and a downstream consumer copies them from there (D-018). The coverage only they gave moved first:

| Coverage | Now |
|---|---|
| Router upstream filters: parsed and rendered (`TestE2E_UpstreamFilters`) | Neutral hand-built dump `testdata/fixtures/upstream-extproc` |
| CLI JSON for `upstreamFilters` (typed config, origin) and per-route `ExtProcPerRoute.disabled` | The same dump, with its golden `testdata/golden/fixtures/upstream-extproc.json` |
| Parser accuracy (`TestParse_*`) | OSS v2.5.0 dumps. The first 443 filter is now `dynamic_modules/simple_mutations`, and the rate-limit key is `ratelimit/kgateway-system/ratelimit` |

Both replacement tests passed before the deletion. Hand-built fixtures use neutral names (`example.transformation`, `AcmePolicy`/`example.com`). The render fixture uses the stock dynamic-module type, so its sides and output are unchanged apart from the name.

## Test counts (top-level tests plus subtests)

| Package | Before | After | Dropped |
|---|---|---|---|
| `cmd/krp` | 89 | 71 | 19 downstream `TestDumpJSON_Golden` cases; +1 for the fixture |
| `pkg/envoy/parser` | 150 | 122 | 19 downstream `TestParseAllScenarios` cases; the downstream case of each of the 9 matcher tests (01_1–01_9) |
| `pkg/envoy/filter` | 97 | 78 | the downstream-scenario filter test and its 18 subtests. `TestFilter_E2E_OSSScenarios` covers the same scenarios on OSS |
| other packages | unchanged | unchanged | — |

Every dropped case ran on a downstream dump. No OSS golden changed.

## Greps
- Scan for vendor names, edition names and an edition-specific suffix in Go under `pkg cmd internal`: only `origin/kgateway.go:295` and `chain/sides.go:34`, which 8.3 removes.
- The same scan over `testdata/`: only `gcr.io/solo-public/docs/hey` in recorded OSS manifests.

## Reviews
Planned sub-phase: reviewed in the phase review before `v0.6.0`.

## Deferred
None.
