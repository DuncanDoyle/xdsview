# Phase 4.3 — Test corpus (Complete)

**Completed:** 2026-11-04
**Range:** 2b0d23f..c0853ef

## Summary
krp is now tested against OSS kgateway. 19 scenarios are recorded on v2.4.7 (Envoy 1.38.7) and v2.5.0 (Envoy 1.39.6), and the route naming convention holds on both. The earlier enterprise recordings remain as a second corpus. The filter e2e test now checks exact route sets, so a dropped or extra route fails it.

## Sub-phases (phase record only)
n/a

## Reviews
- None for this sub-phase: planned sub-phases are reviewed at the phase review.

## Waivers
none

## Deliverables

| Brief row | Evidence |
|---|---|
| Recording on both versions | `hack/record-dumps.sh v2.5.0` and `v2.4.7`: "recorded 19 scenarios" each, exit 0. The kind cluster was deleted afterwards; no leftover processes; `~/.kube/config` untouched (temp kubeconfig). |
| Isolation / settling | One scenario at a time: Accepted waits, two equal `/config_dump` reads containing every HTTPRoute marker, then teardown and a wait for the proxy pod to be deleted |
| Exact route sets per version | `TestFilter_E2E_OSSScenarios`: 38 subtests (19 scenarios × 2 versions) |
| Tests bite | Changing 02_1's expected count to 1 fails (`kept 2 routes, want 1`). Mutating `filter.go` to match `httproute-<name>` without the namespace fails 06 on both versions (`kept a route of another HTTPRoute: ...api-example-com-admin...`). Both reverted. |
| Corpus completeness | `TestFilter_E2E_OSSCorpusComplete`: every scenario with an HTTPRoute has an expectation and both recordings |
| Parser e2e | `TestParseAllScenarios`: 57 dumps (38 OSS + 19 enterprise) with no parse warnings. Matcher tests run on the enterprise dump and both OSS versions (3 runs each). |
| Policies applied | Present in the dumps: 02_1 `dynamic_modules/simple_mutations` (per-route), 02_6 `envoy.filters.http.cors`, 02_7 `ext_auth/kgateway-system/ext-authz`, 02_8 `ratelimit/kgateway-system/ratelimit`, and 06 `ext_proc/kgateway-system/ext-proc-a` and `-b` |
| Enterprise corpus | `testdata/enterprise/` holds the dumps (renamed, unchanged), the original manifests and scripts, and a README naming the E6 removal |
| Golden files | Regenerated: 59 files (38 OSS, 19 enterprise, 2 filtered). The filtered cases now use 06 v2.5.0, where `--route api-example-com` drops `api-example-com-admin`. |
| tests / vet | `go test -count=1 ./...` all ok; `go vet ./...` clean |

## Notes for later phases
- **Phase 5 rendering/mapping:** OSS filter names differ from the enterprise ones, and some are per-extension:
  - `ext_auth/<ns>/<name>` (not `envoy.filters.http.ext_authz`), `ratelimit/<ns>/<name>`, `ext_proc/<ns>/<name>`
  - `global_disable/<kind>` markers
  - transformation as the dynamic module `dynamic_modules/simple_mutations`, not `io.solo.transformation`

  The renderer prints them, but any kind → filter-name mapping must cover them.
- The HTTPS scenario (02_1) names its virtual hosts `https-api~...` and `https-developer~...` after the listener names, not `listener~443`.
- The first recording run failed partway on both versions ("did not settle"). The script killed the subshell around a shell function rather than kubectl, so port-forwards piled up on a 4-CPU Docker VM shared with another cluster. With the fix, a full re-run of both versions passed. The causal link is likely but not proven. The script now writes `*.unsettled` and names the failed condition if it happens again.
- `testdata/scenarios/*/k8s/httpbin.yaml` keeps a `hey` load-generator image from a public docs registry. It's unused by the recordings, and it's worth replacing before the move into kgateway.
- 03, 04 and 05 still have only a `gateway.yaml` and aren't recorded (#17).

## Deferred / open
- #17 multi-route / multi-rule dumps (03, 04, 05) — Phase 5, unchanged
