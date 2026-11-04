# Phase 4.1 — Library move and API (Complete)

**Completed:** 2026-11-04
**Range:** 5ef0306..d672ef8

## Summary
krp's Envoy view is importable from `github.com/DuncanDoyle/krp/pkg/envoy/...`. The packages are `model`, `parser` (`Parse(io.Reader)`), `filter` (`Options`, `AllRules`), `source` (`File`, `Admin`) and `source/kube` (`Gateway`, `Deployment`, with an optional `*rest.Config`). The CLI's flags and output are unchanged.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review (`reviews/phase-codex.md`).

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| CLI output byte-identical | 160 captures compared to the base binary (`5ef0306`): every scenario dump unfiltered, `--route` with `--rule -1/0/1`, and 8 invalid-flag cases. `diff -r` is empty. Run before and after the client-go bump. |
| Parser reader error / trailing data | `TestParse_ReaderError`, `TestParse_TrailingData` pass. These test the new `io.Reader` entry point, so there is no failing "before" state; the trailing-data behaviour matches base `json.Unmarshal`. |
| `source.Admin` cancelled ctx / non-200 | `TestAdmin_CancelledContext`, `TestAdmin_ServerError` |
| kube tunnel closed on Close and every failure | `TestConfigDump_CloseStopsTunnel` (stops once after two Close calls), `TestConfigDump_FailureStopsTunnel` (request error, non-200, cancelled ctx) |
| Pod finders | `TestFindGatewayProxyPod`, `TestFindDeploymentPod` (fake clientset; there were no finder tests before) |
| Import boundary | `pkg/envoy/boundary_test.go` passes. A probe file importing client-go into `source` made it fail (it reported `goautoneg`, `x/text`, …), and the probe was then removed. |
| External consumer | A separate module with `replace` imports `parser`, `filter` and `source`, and parses 00-simple: `listeners=1 filtered=1 warnings=0`. Its only non-stdlib dependencies are the four krp core packages. |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

Other deliverables:
- Dependencies: client-go, api and apimachinery at v0.36.2, with `go 1.26.0`. No new direct dependencies.
- `LICENSE` added (Apache-2.0), and the README updated with the structure, library usage and license link.

## Notes for later phases
- The installed `staticcheck` predates Go 1.27 export data and can't analyse the module. It wasn't run.
- The parser e2e test still imports `internal/renderer`. That's fine inside the module, but it ties a library test to CLI output. Revisit in 4.3 when the e2e tests are rewritten.
- The `internal/` import blocks keep the old order after the path change (`pkg/...` sorts after `internal/...`). That's cosmetic, and was left to keep the diff to path changes only.

## Deferred / open
none
